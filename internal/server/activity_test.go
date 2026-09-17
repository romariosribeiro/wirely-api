package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type fakeWebhookRetrier struct {
	event engine.Event
	err   error
}

func (f *fakeWebhookRetrier) Retry(event engine.Event) error {
	f.event = event
	return f.err
}

func authenticatedCookie(t *testing.T, store *storage.Store, handler http.Handler) *http.Cookie {
	t.Helper()
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) == 0 {
		t.Fatalf("login failed: %d %s", response.Code, response.Body.String())
	}
	return response.Result().Cookies()[0]
}

func TestActivityAPIFiltersAndPaginates(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "History")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.SaveActivityEvent(context.Background(), "evt-message", instance.ID, "message.received", "2026-09-16T10:00:00Z", map[string]any{"id": "msg-1", "text": "Olá"})
	_, _ = store.SaveActivityEvent(context.Background(), "evt-connection", instance.ID, "instance.status", "2026-09-16T10:01:00Z", map[string]any{"status": "connected"})

	app := New(Dependencies{Store: store})
	cookie := authenticatedCookie(t, store, app.Handler())
	request := httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/events?category=messages&page=1&pageSize=1", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list activity failed: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"total":1`) || !strings.Contains(response.Body.String(), `"event":"message.received"`) {
		t.Fatalf("unexpected activity response: %s", response.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/events?category=unknown", nil)
	invalid.AddCookie(cookie)
	invalidResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid filter status 400, got %d", invalidResponse.Code)
	}
}

func TestFailedWebhookDeliveryCanBeRetried(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Retry")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveActivityEvent(context.Background(), "evt-retry", instance.ID, "message.received", "2026-09-16T10:00:00Z", map[string]any{"id": "msg-retry", "text": "Reenviar"})
	if err != nil {
		t.Fatal(err)
	}
	err = store.RecordWebhookDelivery(context.Background(), storage.DeliveryRecord{
		EventID: "evt-retry", InstanceID: instance.ID, Event: "message.received", URL: "https://example.com/hook",
		Attempt: 3, Status: "failed", HTTPStatus: 500, Error: "endpoint returned HTTP 500", Duration: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveries, _, err := store.ListWebhookDeliveries(context.Background(), instance.ID, "failed", 1, 10)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("delivery setup failed: %v %#v", err, deliveries)
	}
	retrier := &fakeWebhookRetrier{}
	app := New(Dependencies{Store: store, Webhooks: retrier})
	cookie := authenticatedCookie(t, store, app.Handler())
	url := "/api/instances/" + instance.ID + "/webhook-deliveries/" + jsonInt(deliveries[0].ID) + "/retry"
	request := httptest.NewRequest(http.MethodPost, url, nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry failed: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"queued"`) {
		t.Fatalf("unexpected retry response: %s", response.Body.String())
	}
	if retrier.event.ID != "evt-retry" || retrier.event.Data["text"] != "Reenviar" {
		t.Fatalf("unexpected retried event: %#v", retrier.event)
	}
}

func TestActivityAPIRequiresAuthentication(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodGet, "/api/instances/example/events", nil)
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func jsonInt(value int64) string {
	data, _ := json.Marshal(value)
	return string(data)
}
