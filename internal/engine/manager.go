package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waCompanionReg "go.mau.fi/whatsmeow/proto/waCompanionReg"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	_ "modernc.org/sqlite"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

var ErrQRUnavailable = errors.New("qr code is not available")
var ErrPairingUnavailable = errors.New("pairing code is not available")

var nonDigits = regexp.MustCompile(`[^0-9]`)

type State struct {
	Status          string `json:"status"`
	QRAvailable     bool   `json:"qrAvailable"`
	QRExpiresAt     string `json:"qrExpiresAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	ConnectionRoute string `json:"connectionRoute,omitempty"`
	ProxyFallback   bool   `json:"proxyFallback,omitempty"`
}

type ContactCheck struct {
	Phone  string `json:"phone"`
	JID    string `json:"jid,omitempty"`
	Exists bool   `json:"exists"`
}

type Manager struct {
	dataDirectory  string
	store          *storage.Store
	mediaDownloads chan struct{}

	mu       sync.RWMutex
	sessions map[string]*session

	eventMu      sync.RWMutex
	eventHandler EventHandler
	presenceMu   sync.RWMutex
	presences    map[string]PresenceState
}
type session struct {
	id        string
	client    *whatsmeow.Client
	container *sqlstore.Container
	cancel    context.CancelFunc

	mu               sync.RWMutex
	status           string
	qrCode           string
	qrExpires        time.Time
	lastError        string
	connecting       bool
	settings         storage.InstanceSettings
	proxyConfigured  bool
	proxyFallback    bool
	reconnectAllowed bool
}

func NewManager(dataDirectory string, store *storage.Store) (*Manager, error) {
	configureDeviceIdentity()
	sessionsDirectory := filepath.Join(dataDirectory, "whatsapp")
	if err := os.MkdirAll(sessionsDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create WhatsApp session directory: %w", err)
	}
	return &Manager{
		dataDirectory:  sessionsDirectory,
		store:          store,
		sessions:       make(map[string]*session),
		mediaDownloads: make(chan struct{}, 2),
		presences:      make(map[string]PresenceState),
	}, nil
}

func configureDeviceIdentity() {
	osName := "Google Chrome"
	platform := waCompanionReg.DeviceProps_CHROME
	whatsmeowStore.DeviceProps.Os = &osName
	whatsmeowStore.DeviceProps.PlatformType = &platform
}

func (m *Manager) Restore(ctx context.Context) error {
	instances, err := m.store.ListInstances(ctx)
	if err != nil {
		return err
	}
	for _, instance := range instances {
		_ = m.store.UpdateInstanceStatus(ctx, instance.ID, "disconnected")
		current, err := m.ensure(ctx, instance.ID)
		if err != nil {
			m.persistStatus(instance.ID, "error")
			continue
		}
		if current.client.Store.ID != nil {
			_ = m.Connect(instance.ID)
		}
	}
	return nil
}

func (m *Manager) Connect(id string) error {
	current, err := m.ensure(context.Background(), id)
	if err != nil {
		return err
	}

	current.mu.Lock()
	if current.client.IsConnected() && current.client.IsLoggedIn() {
		current.mu.Unlock()
		m.setState(current, "connected", "")
		return nil
	}
	if current.connecting {
		current.mu.Unlock()
		return nil
	}
	current.connecting = true
	current.reconnectAllowed = true
	current.lastError = ""
	current.mu.Unlock()
	m.setState(current, "connecting", "")

	go m.connect(current)
	return nil
}

func (m *Manager) connect(current *session) {
	if current.client.Store.ID == nil {
		qrChannel, err := current.client.GetQRChannel(context.Background())
		if err != nil {
			m.connectionError(current, err)
			return
		}
		go m.consumeQR(current, qrChannel)
	}

	if err := m.connectWithProxyFallback(current, current.client.Connect); err != nil {
		m.connectionError(current, err)
	}
}

func (m *Manager) consumeQR(current *session, channel <-chan whatsmeow.QRChannelItem) {
	for item := range channel {
		switch item.Event {
		case whatsmeow.QRChannelEventCode:
			current.mu.Lock()
			current.qrCode = item.Code
			current.qrExpires = time.Now().UTC().Add(item.Timeout)
			current.lastError = ""
			current.mu.Unlock()
			m.setState(current, "qr", "")
		case whatsmeow.QRChannelSuccess.Event:
			current.mu.Lock()
			current.qrCode = ""
			current.qrExpires = time.Time{}
			current.mu.Unlock()
			m.setState(current, "connecting", "")
		case whatsmeow.QRChannelTimeout.Event:
			current.mu.Lock()
			current.connecting = false
			current.qrCode = ""
			current.qrExpires = time.Time{}
			current.mu.Unlock()
			m.setState(current, "disconnected", "")
		case whatsmeow.QRChannelEventError:
			m.connectionError(current, item.Error)
		default:
			if item.Event != "" {
				m.connectionError(current, fmt.Errorf("pairing event: %s", item.Event))
			}
		}
	}
}

func (m *Manager) Disconnect(id string) error {
	current, err := m.get(id)
	if err != nil {
		return err
	}
	current.mu.Lock()
	current.reconnectAllowed = false
	current.mu.Unlock()
	current.client.Disconnect()
	current.mu.Lock()
	current.connecting = false
	current.qrCode = ""
	current.qrExpires = time.Time{}
	current.mu.Unlock()
	m.setState(current, "disconnected", "")
	return nil
}

func (m *Manager) Logout(ctx context.Context, id string) error {
	current, err := m.get(id)
	if err != nil {
		return err
	}
	if !current.client.IsLoggedIn() {
		return ErrNotConnected
	}
	if err := current.client.Logout(ctx); err != nil {
		return fmt.Errorf("logout WhatsApp session: %w", err)
	}
	current.mu.Lock()
	current.reconnectAllowed = false
	current.connecting = false
	current.qrCode = ""
	current.qrExpires = time.Time{}
	current.mu.Unlock()
	m.setState(current, "logged_out", "")
	m.mu.Lock()
	if m.sessions[id] == current {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	current.cancel()
	if err := current.container.Close(); err != nil {
		return fmt.Errorf("close logged out session: %w", err)
	}
	return nil
}

func (m *Manager) PairPhone(ctx context.Context, id, phone string) (string, error) {
	phone = nonDigits.ReplaceAllString(phone, "")
	if len(phone) < 8 || len(phone) > 15 || strings.HasPrefix(phone, "0") {
		return "", errors.New("phone must use international format without +")
	}
	current, err := m.ensure(ctx, id)
	if err != nil {
		return "", err
	}
	if current.client.Store.ID != nil {
		return "", errors.New("instance is already paired")
	}
	if err := m.Connect(id); err != nil {
		return "", err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		current.mu.RLock()
		ready := current.qrCode != "" && time.Now().UTC().Before(current.qrExpires)
		current.mu.RUnlock()
		if ready {
			break
		}
		select {
		case <-waitCtx.Done():
			return "", ErrPairingUnavailable
		case <-ticker.C:
		}
	}
	code, err := current.client.PairPhone(waitCtx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("request pairing code: %w", err)
	}
	return code, nil
}

func (m *Manager) ClearProxy(id string) error {
	return m.SetProxy(id, "")
}

func (m *Manager) SetProxy(id, address string) error {
	current, err := m.get(id)
	if err != nil {
		return nil
	}
	wasConnected := current.client.IsConnected()
	if err := current.client.SetProxyAddress(address); err != nil {
		return fmt.Errorf("configure proxy: %w", err)
	}
	current.mu.Lock()
	current.proxyConfigured = address != ""
	current.proxyFallback = false
	current.mu.Unlock()
	if wasConnected {
		current.client.Disconnect()
		return m.Connect(id)
	}
	return nil
}

func (m *Manager) CheckContacts(ctx context.Context, id string, phones []string) ([]ContactCheck, error) {
	if len(phones) == 0 || len(phones) > 100 {
		return nil, errors.New("provide between 1 and 100 phone numbers")
	}
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	normalized := make([]string, 0, len(phones))
	seen := make(map[string]struct{}, len(phones))
	for _, value := range phones {
		phone := nonDigits.ReplaceAllString(value, "")
		if len(phone) < 8 || len(phone) > 15 || strings.HasPrefix(phone, "0") {
			return nil, fmt.Errorf("invalid phone number %q", value)
		}
		if _, exists := seen[phone]; exists {
			continue
		}
		seen[phone] = struct{}{}
		normalized = append(normalized, phone)
	}
	response, err := current.client.IsOnWhatsApp(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("check WhatsApp contacts: %w", err)
	}
	results := make([]ContactCheck, 0, len(response))
	for _, item := range response {
		phone := nonDigits.ReplaceAllString(item.Query, "")
		if phone == "" && !item.PhoneNumber.IsEmpty() {
			phone = item.PhoneNumber.User
		}
		result := ContactCheck{Phone: phone, Exists: item.IsIn}
		if !item.JID.IsEmpty() {
			result.JID = item.JID.ToNonAD().String()
		}
		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	current := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()

	if current != nil {
		current.mu.Lock()
		current.reconnectAllowed = false
		current.mu.Unlock()
		current.cancel()
		current.client.Disconnect()
		if err := current.container.Close(); err != nil {
			return fmt.Errorf("close WhatsApp session: %w", err)
		}
	}

	databasePath := filepath.Join(m.dataDirectory, id+".db")
	for _, suffix := range []string{"", "-shm", "-wal"} {
		if err := os.Remove(databasePath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove WhatsApp session data: %w", err)
		}
	}
	instanceHash := sha256.Sum256([]byte(id))
	if err := os.RemoveAll(filepath.Join(m.dataDirectory, "received-media", hex.EncodeToString(instanceHash[:16]))); err != nil {
		return fmt.Errorf("remove received media: %w", err)
	}
	return nil
}

func (m *Manager) State(id string) (State, error) {
	current, err := m.get(id)
	if err != nil {
		instance, storageErr := m.store.GetInstance(context.Background(), id)
		if storageErr != nil {
			return State{}, storageErr
		}
		return State{Status: instance.Status}, nil
	}
	current.mu.RLock()
	defer current.mu.RUnlock()
	state := State{
		Status:          current.status,
		QRAvailable:     current.qrCode != "" && time.Now().UTC().Before(current.qrExpires),
		LastError:       current.lastError,
		ConnectionRoute: "direct",
		ProxyFallback:   current.proxyFallback,
	}
	if current.proxyConfigured && !current.proxyFallback {
		state.ConnectionRoute = "proxy"
	}
	if state.Status == "connected" && (!current.client.IsConnected() || !current.client.IsLoggedIn()) {
		state.Status = "connecting"
	}
	if state.QRAvailable {
		state.QRExpiresAt = current.qrExpires.Format(time.RFC3339)
	}
	return state, nil
}

func (m *Manager) JID(id string) string {
	current, err := m.get(id)
	if err != nil || current.client.Store.ID == nil {
		return ""
	}
	return current.client.Store.ID.ToNonAD().String()
}

func (m *Manager) ApplySettings(id string, settings storage.InstanceSettings) error {
	current, err := m.get(id)
	if err != nil {
		return nil // The settings will be loaded when the session is initialized.
	}
	current.mu.Lock()
	current.settings = settings
	connected := current.client.IsConnected() && current.client.IsLoggedIn()
	current.mu.Unlock()
	if connected {
		presence := types.PresenceUnavailable
		if settings.AlwaysOnline {
			presence = types.PresenceAvailable
		}
		if err := current.client.SendPresence(context.Background(), presence); err != nil {
			return fmt.Errorf("apply instance presence: %w", err)
		}
	}
	return nil
}

func (m *Manager) QRCode(id string) ([]byte, error) {
	current, err := m.get(id)
	if err != nil {
		return nil, err
	}
	current.mu.RLock()
	code := current.qrCode
	expires := current.qrExpires
	current.mu.RUnlock()
	if code == "" || time.Now().UTC().After(expires) {
		return nil, ErrQRUnavailable
	}
	png, err := qrcode.Encode(code, qrcode.Medium, 320)
	if err != nil {
		return nil, fmt.Errorf("render QR code: %w", err)
	}
	return png, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = make(map[string]*session)
	m.mu.Unlock()

	var firstError error
	for _, current := range sessions {
		current.mu.Lock()
		current.reconnectAllowed = false
		current.mu.Unlock()
		current.cancel()
		current.client.Disconnect()
		if err := current.container.Close(); err != nil && firstError == nil {
			firstError = err
		}
	}
	return firstError
}

func (m *Manager) ensure(ctx context.Context, id string) (*session, error) {
	if current, err := m.get(id); err == nil {
		return current, nil
	}
	instance, err := m.store.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}

	databasePath := filepath.Join(m.dataDirectory, id+".db")
	dsn := (&url.URL{Scheme: "file", Path: databasePath}).String() +
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open WhatsApp session database: %w", err)
	}
	db.SetMaxOpenConns(1)
	container := sqlstore.NewWithDB(db, "sqlite3", nil)
	if err := container.Upgrade(ctx); err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("upgrade WhatsApp session database: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("load WhatsApp device: %w", err)
	}

	sessionContext, cancel := context.WithCancel(context.Background())
	current := &session{
		id: id, client: whatsmeow.NewClient(device, nil), container: container,
		cancel: cancel, status: "disconnected", settings: instance.InstanceSettings,
	}
	proxyAddress, err := m.store.GetInstanceProxy(ctx, id)
	if err != nil {
		cancel()
		_ = container.Close()
		return nil, err
	}
	if err := current.client.SetProxyAddress(proxyAddress); err != nil {
		cancel()
		_ = container.Close()
		return nil, fmt.Errorf("configure instance proxy: %w", err)
	}
	current.proxyConfigured = proxyAddress != ""
	current.client.BackgroundEventCtx = sessionContext
	// Paired sessions retry startup network failures as well as later outages.
	current.client.InitialAutoReconnect = device.ID != nil
	current.client.AutoReconnectHook = func(err error) bool {
		return m.handleReconnectFailure(current, err)
	}
	current.client.AddEventHandler(func(event any) {
		m.handleEvent(current, event)
	})

	m.mu.Lock()
	if existing := m.sessions[id]; existing != nil {
		m.mu.Unlock()
		cancel()
		_ = container.Close()
		return existing, nil
	}
	m.sessions[id] = current
	m.mu.Unlock()

	go func() {
		<-sessionContext.Done()
	}()
	return current, nil
}

func (m *Manager) get(id string) (*session, error) {
	m.mu.RLock()
	current := m.sessions[id]
	m.mu.RUnlock()
	if current == nil {
		return nil, errors.New("instance session is not initialized")
	}
	return current, nil
}

func (m *Manager) handleEvent(current *session, event any) {
	switch value := event.(type) {
	case *events.Connected:
		current.mu.Lock()
		current.connecting = false
		current.qrCode = ""
		current.qrExpires = time.Time{}
		current.mu.Unlock()
		m.setState(current, "connected", "")
		current.mu.RLock()
		alwaysOnline := current.settings.AlwaysOnline
		current.mu.RUnlock()
		presence := types.PresenceUnavailable
		if alwaysOnline {
			presence = types.PresenceAvailable
		}
		go func() { _ = current.client.SendPresence(context.Background(), presence) }()
	case *events.PairSuccess:
		m.setState(current, "connecting", "")
	case *events.KeepAliveTimeout:
		m.setState(current, "connecting", "WhatsApp keepalive timed out")
	case *events.KeepAliveRestored:
		if current.client.IsConnected() && current.client.IsLoggedIn() {
			m.setState(current, "connected", "")
		}
	case *events.Disconnected:
		current.mu.Lock()
		current.connecting = false
		reconnecting := current.reconnectAllowed && current.client.Store.ID != nil
		current.mu.Unlock()
		if reconnecting {
			m.setState(current, "connecting", "")
		} else {
			m.setState(current, "disconnected", "")
		}
	case *events.LoggedOut:
		current.mu.Lock()
		current.reconnectAllowed = false
		current.connecting = false
		current.qrCode = ""
		current.qrExpires = time.Time{}
		current.mu.Unlock()
		m.setState(current, "logged_out", value.Reason.String())
	case *events.ConnectFailure:
		m.connectionError(current, fmt.Errorf("%s: %s", value.Reason.String(), value.Message))
	case *events.StreamReplaced:
		current.mu.Lock()
		current.reconnectAllowed = false
		current.connecting = false
		current.mu.Unlock()
		m.setState(current, "disconnected", "WhatsApp session replaced by another connection")
	case *events.PairError:
		m.connectionError(current, value.Error)
	case *events.Message:
		current.mu.RLock()
		settings := current.settings
		current.mu.RUnlock()
		if (settings.IgnoreGroups && value.Info.IsGroup) ||
			(settings.IgnoreStatus && value.Info.Chat == types.StatusBroadcastJID) {
			return
		}
		if settings.ReadMessages && !value.Info.IsFromMe {
			go func() {
				sender := value.Info.Sender
				if sender.IsEmpty() {
					sender = value.Info.Chat
				}
				_ = current.client.MarkRead(context.Background(), []types.MessageID{value.Info.ID},
					value.Info.Timestamp, value.Info.Chat, sender)
			}()
		}
		go m.emit(m.prepareIncomingMessage(current, value))
	case *events.Receipt:
		m.emit(receiptEvent(current.id, value))
	case *events.Presence:
		m.rememberPresence(current.id, value)
		m.emit(presenceEvent(current.id, value))
	case *events.ChatPresence:
		m.emit(chatPresenceEvent(current.id, value))
	case *events.GroupInfo:
		m.emit(groupEvent(current.id, value))
	case *events.HistorySync:
		data := map[string]any{}
		if value.Data != nil {
			data["type"] = value.Data.GetSyncType().String()
			data["progress"] = value.Data.GetProgress()
			data["chunkOrder"] = value.Data.GetChunkOrder()
			data["conversations"] = len(value.Data.GetConversations())
		}
		m.emit(newEvent("history.sync", current.id, time.Now().UTC(), data))
	case *events.CallOffer:
		m.emit(callEvent(current.id, "call.offer", value.BasicCallMeta, value.RemotePlatform, value.RemoteVersion, ""))
		current.mu.RLock()
		settings := current.settings
		current.mu.RUnlock()
		if settings.RejectCall {
			go func() {
				caller := value.From
				if caller.IsEmpty() {
					caller = value.CallCreator
				}
				if err := current.client.RejectCall(context.Background(), caller, value.CallID); err != nil {
					return
				}
				if strings.TrimSpace(settings.MsgRejectCall) != "" {
					_, _ = m.SendText(context.Background(), current.id, caller.ToNonAD().String(), settings.MsgRejectCall)
				}
			}()
		}
	case *events.CallAccept:
		m.emit(callEvent(current.id, "call.accept", value.BasicCallMeta, value.RemotePlatform, value.RemoteVersion, ""))
	case *events.CallReject:
		m.emit(callEvent(current.id, "call.reject", value.BasicCallMeta, "", "", ""))
	case *events.CallTerminate:
		m.emit(callEvent(current.id, "call.terminate", value.BasicCallMeta, "", "", value.Reason))
	case *events.Contact:
		data := map[string]any{"jid": value.JID.ToNonAD().String(), "fromFullSync": value.FromFullSync}
		if value.Action != nil {
			data["fullName"] = value.Action.GetFullName()
			data["firstName"] = value.Action.GetFirstName()
			data["username"] = value.Action.GetUsername()
		}
		m.emit(newEvent("contact.updated", current.id, value.Timestamp, data))
	case *events.LabelEdit:
		data := map[string]any{"labelId": value.LabelID, "fromFullSync": value.FromFullSync}
		if value.Action != nil {
			data["name"] = value.Action.GetName()
			data["color"] = value.Action.GetColor()
			data["deleted"] = value.Action.GetDeleted()
		}
		m.emit(newEvent("label.updated", current.id, value.Timestamp, data))
	case *events.LabelAssociationChat:
		data := map[string]any{"chat": value.JID.ToNonAD().String(), "labelId": value.LabelID, "fromFullSync": value.FromFullSync}
		if value.Action != nil {
			data["labeled"] = value.Action.GetLabeled()
		}
		m.emit(newEvent("label.chat", current.id, value.Timestamp, data))
	case *events.LabelAssociationMessage:
		data := map[string]any{"chat": value.JID.ToNonAD().String(), "messageId": value.MessageID, "labelId": value.LabelID, "fromFullSync": value.FromFullSync}
		if value.Action != nil {
			data["labeled"] = value.Action.GetLabeled()
		}
		m.emit(newEvent("label.message", current.id, value.Timestamp, data))
	case *events.NewsletterJoin:
		data := map[string]any{"jid": value.ID.String(), "name": value.ThreadMeta.Name.Text}
		if value.ViewerMeta != nil {
			data["role"] = value.ViewerMeta.Role
		}
		m.emit(newEvent("newsletter.join", current.id, value.Mex.Timestamp, data))
	case *events.NewsletterLeave:
		m.emit(newEvent("newsletter.leave", current.id, value.Mex.Timestamp, map[string]any{"jid": value.ID.String(), "role": value.Role}))
	case *events.NewsletterMuteChange:
		m.emit(newEvent("newsletter.mute", current.id, value.Mex.Timestamp, map[string]any{"jid": value.ID.String(), "mute": value.Mute}))
	case *events.NewsletterLiveUpdate:
		m.emit(newEvent("newsletter.live_update", current.id, value.Time, map[string]any{"jid": value.JID.String(), "messages": len(value.Messages)}))
	}
}

func (m *Manager) connectionError(current *session, err error) {
	message := "unknown connection error"
	if err != nil {
		message = err.Error()
	}
	current.mu.Lock()
	current.connecting = false
	current.qrCode = ""
	current.qrExpires = time.Time{}
	current.mu.Unlock()
	m.setState(current, "error", message)
}

func (m *Manager) setState(current *session, status, lastError string) {
	current.mu.Lock()
	changed := current.status != status || current.lastError != lastError
	current.status = status
	current.lastError = lastError
	current.mu.Unlock()
	m.persistStatus(current.id, status)
	if changed {
		slog.Info("WhatsApp instance state changed", "instance_id", current.id, "status", status)
		m.emit(newEvent("instance.status", current.id, time.Now().UTC(), map[string]any{
			"status": status, "lastError": lastError,
		}))
	}

}
func (m *Manager) persistStatus(id, status string) {

	_ = m.store.UpdateInstanceStatus(context.Background(), id, status)
}
