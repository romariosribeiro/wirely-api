package server

import (
	"context"
	"encoding/json"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookPreferencesAPI(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "API preferences")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := store.CreateSession(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	endpoint := "/api/v1/instances/" + instance.ID + "/webhook"
	send := func(method, body string, authenticated bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, endpoint, strings.NewReader(body))
		if authenticated {
			request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		}
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response
	}
	for _, method := range []string{"GET", "PUT"} {
		if response := send(method, "{}", false); response.Code != 401 {
			t.Fatal("webhook route is not protected")
		}
	}
	body := `{"url":"https://example.com/events","enabled":true,"events":["messages","presence"]}`
	response := send("PUT", body, true)
	if response.Code != 200 {
		t.Fatalf("save failed: %s", response.Body.String())
	}
	var created storage.WebhookConfig
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" || !created.Enabled || len(created.Events) != 2 {
		t.Fatal("invalid saved preferences")
	}
	response = send("PUT", `{"url":"https://example.com/events","enabled":false,"events":["groups"]}`, true)
	if response.Code != 200 || strings.Contains(response.Body.String(), `"secret":`) {
		t.Fatal("saving selection rotated secret")
	}
	response = send("GET", "", true)
	var loaded storage.WebhookConfig
	if err := json.Unmarshal(response.Body.Bytes(), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Enabled || loaded.URL != created.URL || len(loaded.Events) != 1 || loaded.Events[0] != "groups" || loaded.Secret != "" {
		t.Fatal("preferences not persisted")
	}
	response = send("PUT", `{"url":"https://example.com/events","enabled":true,"events":[]}`, true)
	if response.Code != 200 {
		t.Fatal("empty selection rejected")
	}
	for _, invalid := range []string{`{"url":"","enabled":true,"events":[]}`, `{"url":"https://example.com/events","enabled":true,"events":["invalid"]}`} {
		if response := send("PUT", invalid, true); response.Code != 422 {
			t.Fatalf("invalid config accepted: %s", response.Body.String())
		}
	}
	if response := send("PUT", body, true); response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credentials can be cached")
	}
}
