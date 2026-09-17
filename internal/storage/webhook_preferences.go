package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/romariosribeiro/wirely-api/internal/security"
	"slices"
	"time"
)

var ErrWebhookEvents = errors.New("invalid webhook event selection")
var ErrWebhookURLRequired = errors.New("webhook URL is required when enabled")

func (target WebhookTarget) Allows(event string) bool {
	if !target.Enabled {
		return false
	}
	if event == "webhook.test" {
		return true
	}
	category := ""
	switch event {
	case "message.received", "message.sent", "message.updated", "message.deleted", "message.reaction", "message.receipt":
		category = "messages"
	case "instance.status":
		category = "connection"
	case "status.received", "status.sent":
		category = "status"
	case "presence.updated", "presence.chat":
		category = "presence"
	case "group.updated":
		category = "groups"
	case "history.sync":
		category = "history"
	case "call.offer", "call.accept", "call.reject", "call.terminate":
		category = "calls"
	case "label.updated", "label.chat", "label.message":
		category = "labels"
	case "contact.updated":
		category = "contacts"
	case "newsletter.join", "newsletter.leave", "newsletter.mute", "newsletter.live_update":
		category = "newsletters"
	}
	return category != "" && slices.Contains(target.Events, category)
}

// Changing preferences preserves the secret. A new destination or explicit
// rotation generates a new secret. Disabling preserves the URL and selections.
func (s *Store) SaveWebhook(ctx context.Context, id, webhookURL string, enabled bool, events []string, rotate bool) (WebhookConfig, error) {
	selected := []string{}
	for _, event := range events {
		switch event {
		case "messages", "connection", "status", "presence", "groups", "history", "calls", "labels", "contacts", "newsletters":
		default:
			return WebhookConfig{}, ErrWebhookEvents
		}
		if !slices.Contains(selected, event) {
			selected = append(selected, event)
		}
	}
	if enabled && webhookURL == "" {
		return WebhookConfig{}, ErrWebhookURLRequired
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return WebhookConfig{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WebhookConfig{}, err
	}
	defer tx.Rollback()
	var oldURL, secret string
	err = tx.QueryRowContext(ctx, "SELECT webhook_url, webhook_secret FROM instances WHERE id = ?", id).Scan(&oldURL, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return WebhookConfig{}, ErrInstanceNotFound
	}
	if err != nil {
		return WebhookConfig{}, err
	}
	displayed := ""
	if webhookURL == "" {
		secret = ""
	} else if secret == "" || oldURL != webhookURL || rotate {
		value, err := security.RandomToken(32)
		if err != nil {
			return WebhookConfig{}, err
		}
		secret = "whsec_" + value
		displayed = secret
	}
	_, err = tx.ExecContext(ctx, "UPDATE instances SET webhook_url = ?, webhook_secret = ?, webhook_enabled = ?, webhook_events = ?, updated_at = ? WHERE id = ?",
		webhookURL, secret, enabled, string(raw), time.Now().UTC().Unix(), id)
	if err != nil {
		return WebhookConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return WebhookConfig{}, err
	}
	return WebhookConfig{URL: webhookURL, Enabled: enabled, HasSecret: secret != "", Secret: displayed, Events: selected}, nil
}
