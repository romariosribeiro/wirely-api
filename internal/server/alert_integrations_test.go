package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func TestAlertIntegrationsSaveAndPreserveSecrets(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Alert tests")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	cookie := authenticatedCookie(t, store, app.Handler())
	payload := map[string]any{
		"telegram": map[string]any{
			"enabled": true, "token": "123456:test-bot-secret", "clearToken": false,
			"chatId": "-100123", "instanceIds": []string{instance.ID},
		},
		"smtp": map[string]any{
			"enabled": true, "host": "smtp.example.com", "port": 465, "security": "tls",
			"username": "alerts@example.com", "password": "test-smtp-secret", "clearPassword": false,
			"from": "alerts@example.com", "recipient": "owner@example.com", "instanceIds": []string{instance.ID},
		},
	}
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			payload["telegram"].(map[string]any)["token"] = ""
			payload["smtp"].(map[string]any)["password"] = ""
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		response := requestWithCookie(app.Handler(), http.MethodPut, "/api/alert-integrations", string(body), cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("save %d failed: %d %s", attempt, response.Code, response.Body.String())
		}
		var saved storage.AlertIntegrations
		if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		if !saved.Telegram.HasToken || !saved.SMTP.HasPassword || saved.SMTP.Port != 465 || saved.SMTP.Security != "tls" ||
			len(saved.Telegram.InstanceIDs) != 1 || len(saved.SMTP.InstanceIDs) != 1 {
			t.Fatalf("save %d lost integration settings", attempt)
		}
		if strings.Contains(response.Body.String(), "test-bot-secret") || strings.Contains(response.Body.String(), "test-smtp-secret") {
			t.Fatal("API response exposed integration credentials")
		}
		targets, err := store.AlertTargets(context.Background())
		if err != nil || targets.Token != "123456:test-bot-secret" || targets.Password != "test-smtp-secret" {
			t.Fatalf("save %d did not preserve credentials", attempt)
		}
	}
}
