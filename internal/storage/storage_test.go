package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceTokenLifecycle(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	instance, err := store.CreateInstance(context.Background(), "Principal")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(instance.APIToken, "wly_") {
		t.Fatalf("expected a Wirely token, got %q", instance.APIToken)
	}

	authenticated, valid, err := store.AuthenticateInstanceToken(context.Background(), instance.APIToken)
	if err != nil || !valid {
		t.Fatalf("expected token to authenticate: valid=%v err=%v", valid, err)
	}
	if authenticated.ID != instance.ID {
		t.Fatalf("token resolved instance %q, want %q", authenticated.ID, instance.ID)
	}

	list, err := store.ListInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].APIToken != "" {
		t.Fatal("instance list must not expose API tokens")
	}

	oldToken := instance.APIToken
	newToken, err := store.RotateInstanceToken(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newToken == oldToken {
		t.Fatal("rotated token must be different")
	}
	if _, valid, err := store.AuthenticateInstanceToken(context.Background(), oldToken); err != nil || valid {
		t.Fatalf("old token should be invalid: valid=%v err=%v", valid, err)
	}
	if _, valid, err := store.AuthenticateInstanceToken(context.Background(), newToken); err != nil || !valid {
		t.Fatalf("new token should be valid: valid=%v err=%v", valid, err)
	}
}

func TestAlertIntegrationSecretsAreEncryptedAndPreserved(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	instance, err := store.CreateInstance(context.Background(), "Alerts")
	if err != nil {
		t.Fatal(err)
	}
	var update AlertIntegrationsUpdate
	update.Telegram.Enabled = true
	update.Telegram.Token = "123456:telegram-secret"
	update.Telegram.ChatID = "-100123"
	update.Telegram.InstanceIDs = []string{instance.ID}
	update.SMTP.Enabled = true
	update.SMTP.Host = "smtp.example.com"
	update.SMTP.Port = 587
	update.SMTP.Security = "starttls"
	update.SMTP.Username = "alerts@example.com"
	update.SMTP.Password = "smtp-secret"
	update.SMTP.From = "Wirely <alerts@example.com>"
	update.SMTP.Recipient = "owner@example.com"
	update.SMTP.InstanceIDs = []string{instance.ID}
	config, err := store.SaveAlertIntegrations(context.Background(), update)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Telegram.HasToken || !config.SMTP.HasPassword {
		t.Fatalf("missing credential flags: %#v", config)
	}
	var raw string
	if err := store.db.QueryRow("SELECT value FROM settings WHERE key = ?", alertIntegrationsKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "telegram-secret") || strings.Contains(raw, "smtp-secret") {
		t.Fatal("alert secrets must not be stored as plaintext")
	}
	targets, err := store.AlertTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if targets.Token != update.Telegram.Token || targets.Password != update.SMTP.Password {
		t.Fatal("encrypted alert secrets did not round trip")
	}
	update.Telegram.Token = ""
	update.SMTP.Password = ""
	if _, err := store.SaveAlertIntegrations(context.Background(), update); err != nil {
		t.Fatal(err)
	}
	targets, err = store.AlertTargets(context.Background())
	if err != nil || targets.Token != "123456:telegram-secret" || targets.Password != "smtp-secret" {
		t.Fatal("blank updates must preserve saved credentials")
	}
}

func TestDefaultAlertIntegrationsUseEmptyInstanceLists(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	config, err := store.GetAlertIntegrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Telegram.InstanceIDs == nil || config.SMTP.InstanceIDs == nil {
		t.Fatalf("instance lists must be empty arrays, not null: %#v", config)
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"instanceIds":null`) {
		t.Fatalf("API response contains a null instance list: %s", body)
	}
}

func TestMessageStatusUsesLatestReceipt(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	instance, err := store.CreateInstance(context.Background(), "Receipts")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.SaveActivityEvent(context.Background(), "evt-sent", instance.ID, "message.sent", "2026-09-17T10:00:00Z", map[string]any{"id": "MSG-1", "chat": "5511999999999@s.whatsapp.net"})
	_, _ = store.SaveActivityEvent(context.Background(), "evt-delivered", instance.ID, "message.receipt", "2026-09-17T10:01:00Z", map[string]any{"ids": []string{"MSG-1"}, "chat": "5511999999999@s.whatsapp.net", "type": ""})
	_, _ = store.SaveActivityEvent(context.Background(), "evt-read", instance.ID, "message.receipt", "2026-09-17T10:02:00Z", map[string]any{"ids": []string{"MSG-1"}, "chat": "5511999999999@s.whatsapp.net", "type": "read"})
	status, err := store.GetMessageStatus(context.Background(), instance.ID, "MSG-1")
	if err != nil || status.Status != "read" || status.Chat == "" {
		t.Fatalf("unexpected status: %#v err=%v", status, err)
	}
	if _, err := store.GetMessageStatus(context.Background(), instance.ID, "missing"); err != ErrMessageStatusMissing {
		t.Fatalf("unexpected missing error: %v", err)
	}
}

func TestWebhookLifecycle(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	instance, err := store.CreateInstance(context.Background(), "Webhook")
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.GetWebhookConfig(context.Background(), instance.ID)
	if err != nil || config.Enabled || config.HasSecret || config.Secret != "" {
		t.Fatalf("expected disabled webhook, got %#v err=%v", config, err)
	}

	configured, err := store.SetWebhook(context.Background(), instance.ID, "https://example.com/wirely")
	if err != nil {
		t.Fatal(err)
	}
	if !configured.Enabled || !configured.HasSecret || !strings.HasPrefix(configured.Secret, "whsec_") {
		t.Fatalf("unexpected configured webhook: %#v", configured)
	}
	readBack, err := store.GetWebhookConfig(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if readBack.URL != configured.URL || !readBack.Enabled || !readBack.HasSecret || readBack.Secret != "" {
		t.Fatalf("webhook secret must only be returned when generated: %#v", readBack)
	}
	target, err := store.GetWebhookTarget(context.Background(), instance.ID)
	if err != nil || target.URL != configured.URL || target.Secret != configured.Secret {
		t.Fatalf("unexpected delivery target: %#v err=%v", target, err)
	}

	rotated, err := store.SetWebhook(context.Background(), instance.ID, "https://example.com/updated")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Secret == configured.Secret {
		t.Fatal("saving a webhook must rotate its signing secret")
	}

	if _, err := store.SetWebhook(context.Background(), instance.ID, ""); err != nil {
		t.Fatal(err)
	}
	disabled, err := store.GetWebhookTarget(context.Background(), instance.ID)
	if err != nil || disabled.URL != "" || disabled.Secret != "" {
		t.Fatalf("expected disabled webhook target, got %#v err=%v", disabled, err)

	}
}
func TestWebhookMigrationFromPreviousSchema(t *testing.T) {
	directory := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(directory, "wirely.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    engine TEXT NOT NULL DEFAULT 'whatsmeow',
    status TEXT NOT NULL DEFAULT 'disconnected',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    api_token_hash TEXT UNIQUE
);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	instance, err := store.CreateInstance(context.Background(), "Migrated")
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.SetWebhook(context.Background(), instance.ID, "https://example.com/events")
	if err != nil || !config.Enabled || config.Secret == "" {
		t.Fatalf("webhook columns were not migrated: %#v err=%v", config, err)
	}
}
