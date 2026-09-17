package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type connectRequest struct {
	Subscribe  *[]string `json:"subscribe,omitempty"`
	WebhookURL *string   `json:"webhookUrl,omitempty"`
	Phone      string    `json:"phone,omitempty"`
}

const connectPairingWait = 30 * time.Second

var errConnectWebhookURL = errors.New("invalid connect webhook URL")

type connectSubscription struct {
	Name     string
	Category string
}

var connectSubscriptions = []connectSubscription{
	{Name: "MESSAGE", Category: "messages"},
	{Name: "SEND_MESSAGE", Category: "messages"},
	{Name: "READ_RECEIPT", Category: "messages"},
	{Name: "PRESENCE", Category: "presence"},
	{Name: "HISTORY_SYNC", Category: "history"},
	{Name: "CHAT_PRESENCE", Category: "presence"},
	{Name: "CALL", Category: "calls"},
	{Name: "CONNECTION", Category: "connection"},
	{Name: "LABEL", Category: "labels"},
	{Name: "CONTACT", Category: "contacts"},
	{Name: "GROUP", Category: "groups"},
	{Name: "NEWSLETTER", Category: "newsletters"},
	{Name: "QRCODE", Category: "connection"},
}

func (s *Server) publicGetInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	instance, err := s.store.GetInstance(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	writeJSON(w, http.StatusOK, instance)
}

func (s *Server) publicConnectInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.connector == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	payload := connectRequest{}
	if err := decodeJSON(w, r, &payload); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	eventNames, webhookURL, err := s.configureConnectWebhook(r, instanceID, payload)
	if errors.Is(err, storage.ErrWebhookEvents) || errors.Is(err, storage.ErrWebhookURLRequired) || errors.Is(err, errConnectWebhookURL) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to configure webhook")
		return
	}
	data := map[string]any{
		"eventString": strings.Join(eventNames, ","),
		"webhookUrl":  webhookURL,
	}
	if strings.TrimSpace(payload.Phone) != "" {
		pairer, ok := s.connector.(InstancePhonePairer)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "phone pairing is unavailable")
			return
		}
		code, pairErr := pairer.PairPhone(r.Context(), instanceID, payload.Phone)
		if errors.Is(pairErr, engine.ErrPairingUnavailable) {
			writeError(w, http.StatusConflict, pairErr.Error())
			return
		}
		if pairErr != nil {
			writeError(w, http.StatusUnprocessableEntity, pairErr.Error())
			return
		}
		data["status"] = "pairing"
		data["pairingCode"] = code
		data["expiresIn"] = 160
	} else {
		if err := s.connector.Connect(instanceID); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.populateConnectQR(r.Context(), instanceID, data)
	}
	jid := ""
	if provider, ok := s.connector.(InstanceJIDProvider); ok {
		jid = provider.JID(instanceID)
	}
	data["jid"] = jid
	writeJSON(w, http.StatusOK, map[string]any{
		"data":    data,
		"message": "success",
	})
}

func (s *Server) populateConnectQR(ctx context.Context, instanceID string, data map[string]any) {
	provider, ok := s.connector.(InstanceQRProvider)
	if !ok {
		return
	}
	waitCtx, cancel := context.WithTimeout(ctx, connectPairingWait)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, err := provider.State(instanceID)
		if err != nil {
			data["status"] = "error"
			data["lastError"] = err.Error()
			return
		}
		data["status"] = state.Status
		data["qrAvailable"] = state.QRAvailable
		if state.QRExpiresAt != "" {
			data["qrExpiresAt"] = state.QRExpiresAt
		}
		if state.LastError != "" {
			data["lastError"] = state.LastError
		}
		if state.QRAvailable {
			png, qrErr := provider.QRCode(instanceID)
			if qrErr == nil {
				data["qrCode"] = map[string]string{
					"mimetype": "image/png",
					"data":     base64.StdEncoding.EncodeToString(png),
				}
			}
			return
		}
		if state.Status == "connected" || state.Status == "error" || state.Status == "disconnected" || state.Status == "logged_out" {
			return
		}
		select {
		case <-waitCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) configureConnectWebhook(r *http.Request, instanceID string, payload connectRequest) ([]string, string, error) {
	current, err := s.store.GetWebhookConfig(r.Context(), instanceID)
	if err != nil {
		return nil, "", err
	}
	eventNames := subscriptionNamesForCategories(current.Events)
	categories := append([]string(nil), current.Events...)
	webhookURL := current.URL
	shouldSave := payload.Subscribe != nil || payload.WebhookURL != nil

	if payload.WebhookURL != nil {
		webhookURL, err = validateWebhookURL(*payload.WebhookURL)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", errConnectWebhookURL, err)
		}
	}
	if payload.Subscribe != nil {
		eventNames, categories, err = normalizeConnectSubscriptions(*payload.Subscribe)
		if err != nil {
			return nil, "", err
		}
	} else if payload.WebhookURL != nil && webhookURL != "" {
		eventNames, categories, _ = normalizeConnectSubscriptions(allConnectSubscriptionNames())
	}
	if shouldSave {
		if len(eventNames) > 0 && webhookURL == "" {
			return nil, "", storage.ErrWebhookURLRequired
		}
		_, err = s.store.SaveWebhook(r.Context(), instanceID, webhookURL, webhookURL != "", categories, false)
		if err != nil {
			return nil, "", err
		}
	}
	return eventNames, webhookURL, nil
}

func normalizeConnectSubscriptions(values []string) ([]string, []string, error) {
	if len(values) > len(connectSubscriptions) {
		return nil, nil, storage.ErrWebhookEvents
	}
	names := make([]string, 0, len(values))
	categories := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.ToUpper(strings.TrimSpace(value))
		found := false
		for _, subscription := range connectSubscriptions {
			if subscription.Name != name {
				continue
			}
			found = true
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
			if !slices.Contains(categories, subscription.Category) {
				categories = append(categories, subscription.Category)
			}
			break
		}
		if !found {
			return nil, nil, storage.ErrWebhookEvents
		}
	}
	return names, categories, nil
}

func subscriptionNamesForCategories(categories []string) []string {
	names := make([]string, 0, len(connectSubscriptions))
	for _, subscription := range connectSubscriptions {
		if slices.Contains(categories, subscription.Category) {
			names = append(names, subscription.Name)
		}
	}
	return names
}

func allConnectSubscriptionNames() []string {
	names := make([]string, len(connectSubscriptions))
	for index, subscription := range connectSubscriptions {
		names[index] = subscription.Name
	}
	return names
}

func (s *Server) publicDisconnectInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Disconnect(instanceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicLogoutInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Logout(r.Context(), instanceID); err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, engine.ErrNotConnected) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicPairInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Phone string `json:"phone"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	code, err := s.engine.PairPhone(r.Context(), instanceID, payload.Phone)
	if errors.Is(err, engine.ErrPairingUnavailable) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expiresIn": 160})
}

func (s *Server) publicDeleteInstanceProxy(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.ClearProxy(instanceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicInstanceQR(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	png, err := s.engine.QRCode(instanceID)
	if errors.Is(err, engine.ErrQRUnavailable) {
		writeError(w, http.StatusNotFound, "QR code is not available")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render QR code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("format") == "base64" {
		writeJSON(w, http.StatusOK, map[string]string{"mimetype": "image/png", "data": base64.StdEncoding.EncodeToString(png)})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) publicInstanceStatus(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	state, err := s.engine.State(instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) publicCheckContacts(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Phones []string `json:"phones"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.engine.CheckContacts(r.Context(), instanceID, payload.Phones)
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
