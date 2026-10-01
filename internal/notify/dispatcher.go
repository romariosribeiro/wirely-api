package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type alertStore interface {
	AlertTargets(context.Context) (storage.AlertTargets, error)
	GetInstance(context.Context, string) (storage.Instance, error)
}

type Dispatcher struct {
	store       alertStore
	httpClient  *http.Client
	telegramURL func(string) string
	queue       chan engine.Event
	done        chan struct{}
	once        sync.Once
	mu          sync.Mutex
	unhealthy   map[string]bool
}

func NewDispatcher(store alertStore) *Dispatcher {
	return &Dispatcher{
		store:       store,
		httpClient:  &http.Client{Timeout: 12 * time.Second},
		telegramURL: func(token string) string { return "https://api.telegram.org/bot" + token + "/sendMessage" },
		queue:       make(chan engine.Event, 100), done: make(chan struct{}), unhealthy: make(map[string]bool),
	}
}

func (d *Dispatcher) Start() { go d.run() }

func (d *Dispatcher) Close() { d.once.Do(func() { close(d.done) }) }

func (d *Dispatcher) Dispatch(event engine.Event) {
	if event.Event != "instance.status" {
		return
	}
	select {
	case d.queue <- event:
	default:
		slog.Warn("alert notification queue is full", "event_id", event.ID, "instance_id", event.InstanceID)
	}
}

func (d *Dispatcher) run() {
	for {
		select {
		case event := <-d.queue:
			d.process(event)
		case <-d.done:
			return
		}
	}
}

func (d *Dispatcher) process(event engine.Event) {
	status, _ := event.Data["status"].(string)
	status = strings.ToLower(strings.TrimSpace(status))
	unhealthy := status == "disconnected" || status == "error" || status == "logged_out"
	d.mu.Lock()
	wasUnhealthy := d.unhealthy[event.InstanceID]
	if unhealthy {
		d.unhealthy[event.InstanceID] = true
	} else if status == "connected" {
		delete(d.unhealthy, event.InstanceID)
	}
	d.mu.Unlock()
	if !unhealthy && !(status == "connected" && wasUnhealthy) {
		return
	}
	instance, err := d.store.GetInstance(context.Background(), event.InstanceID)
	if err != nil {
		slog.Error("failed to load instance for alert", "instance_id", event.InstanceID, "error", err)
		return
	}
	message := statusMessage(instance.Name, status, event.Data)
	if err := d.sendConfigured(context.Background(), event.InstanceID, message); err != nil {
		slog.Error("failed to deliver instance alert", "instance_id", event.InstanceID, "status", status, "error", err)
	}
}

func statusMessage(name, status string, data map[string]any) string {
	now := time.Now().Format("02/01/2006 15:04:05")
	switch status {
	case "connected":
		return fmt.Sprintf("✅ Wirely: a instância %q voltou a ficar conectada.\nHorário: %s", name, now)
	case "logged_out":
		return fmt.Sprintf("🚨 Wirely: a instância %q foi desconectada do WhatsApp (logout).\nHorário: %s", name, now)
	case "error":
		reason, _ := data["lastError"].(string)
		reason = strings.TrimSpace(reason)
		if reason != "" {
			return fmt.Sprintf("🚨 Wirely: a instância %q apresentou erro.\nMotivo: %s\nHorário: %s", name, reason, now)
		}
		return fmt.Sprintf("🚨 Wirely: a instância %q apresentou erro.\nHorário: %s", name, now)
	default:
		return fmt.Sprintf("⚠️ Wirely: a instância %q está desconectada.\nHorário: %s", name, now)
	}
}

func (d *Dispatcher) sendConfigured(ctx context.Context, instanceID, message string) error {
	targets, err := d.store.AlertTargets(ctx)
	if err != nil {
		return err
	}
	var failures []error
	if targets.Telegram.Enabled && contains(targets.Telegram.InstanceIDs, instanceID) {
		if err := d.sendTelegram(ctx, targets.Token, targets.Telegram.ChatID, message); err != nil {
			failures = append(failures, fmt.Errorf("Telegram: %w", err))
		}
	}
	if targets.SMTP.Enabled && contains(targets.SMTP.InstanceIDs, instanceID) {
		if err := sendSMTP(ctx, targets.SMTP, targets.Password, "Alerta de instância", message); err != nil {
			failures = append(failures, fmt.Errorf("SMTP: %w", err))
		}
	}
	return errors.Join(failures...)
}

func (d *Dispatcher) TestTelegram(ctx context.Context) error {
	targets, err := d.store.AlertTargets(ctx)
	if err != nil {
		return err
	}
	if !targets.Telegram.Enabled {
		return errors.New("Telegram alerts are disabled")
	}
	return d.sendTelegram(ctx, targets.Token, targets.Telegram.ChatID, "✅ Teste do Wirely: os alertas pelo Telegram estão funcionando.")
}

func (d *Dispatcher) TestSMTP(ctx context.Context) error {
	targets, err := d.store.AlertTargets(ctx)
	if err != nil {
		return err
	}
	if !targets.SMTP.Enabled {
		return errors.New("SMTP alerts are disabled")
	}
	return sendSMTP(ctx, targets.SMTP, targets.Password, "Teste de alerta", "Teste do Wirely: os alertas por e-mail estão funcionando.")
}

func (d *Dispatcher) sendTelegram(ctx context.Context, token, chatID, message string) error {
	if token == "" || chatID == "" {
		return errors.New("Telegram credentials are incomplete")
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": message, "disable_web_page_preview": true})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.telegramURL(token), bytes.NewReader(payload))
	if err != nil {
		return errors.New("could not prepare Telegram request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := d.httpClient.Do(request)
	if err != nil {
		return errors.New("Telegram request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Telegram returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendSMTP(ctx context.Context, config storage.SMTPAlertConfig, password, subject, body string) error {
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := &net.Dialer{Timeout: 12 * time.Second}
	var client *smtp.Client
	var connection net.Conn
	var err error
	if config.Security == "tls" {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return errors.New("could not connect to SMTP over TLS")
		}
		client, err = smtp.NewClient(connection, config.Host)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return errors.New("could not connect to SMTP server")
		}
		client, err = smtp.NewClient(connection, config.Host)
		if err == nil && config.Security == "starttls" {
			err = client.StartTLS(&tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
		}
	}
	if err != nil {
		return errors.New("could not establish a secure SMTP session")
	}
	defer client.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(15 * time.Second))
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, password, config.Host)); err != nil {
			return errors.New("SMTP authentication failed")
		}
	}
	from, _ := mail.ParseAddress(config.From)
	recipient, _ := mail.ParseAddress(config.Recipient)
	if err := client.Mail(from.Address); err != nil {
		return errors.New("SMTP rejected the sender")
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return errors.New("SMTP rejected the recipient")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("SMTP rejected the message")
	}
	message := buildEmail(config.From, config.Recipient, subject, body)
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return errors.New("could not send SMTP message")
	}
	if err := writer.Close(); err != nil {
		return errors.New("SMTP delivery failed")
	}
	if err := client.Quit(); err != nil {
		return errors.New("SMTP server did not confirm delivery")
	}
	return nil
}

func buildEmail(from, to, subject, body string) []byte {
	clean := func(value string) string { return strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "") }
	headers := "From: " + clean(from) + "\r\nTo: " + clean(to) + "\r\nSubject: " + clean(subject) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n"
	return []byte(headers + strings.ReplaceAll(body, "\n", "\r\n"))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
