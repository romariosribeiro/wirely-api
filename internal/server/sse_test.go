package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/stream"
)

func TestSSEIsAuthenticatedFilteredAndStreamsEvents(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "SSE")
	if err != nil {
		t.Fatal(err)
	}
	hub := stream.New()
	app := New(Dependencies{Store: store, Events: hub})

	unauthorized := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/events", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}

	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/api/events?events=message.received", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { app.Handler().ServeHTTP(response, request); close(done) }()
	time.Sleep(20 * time.Millisecond)
	hub.Publish(engine.Event{ID: "evt_skip", Event: "presence.updated", InstanceID: instance.ID, Timestamp: time.Now().UTC().Format(time.RFC3339), Data: map[string]any{}})
	hub.Publish(engine.Event{ID: "evt_keep", Event: "message.received", InstanceID: instance.ID, Timestamp: time.Now().UTC().Format(time.RFC3339), Data: map[string]any{"text": "hello"}})
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}
	body := response.Body.String()
	if response.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(body, "id: evt_keep") || strings.Contains(body, "evt_skip") {
		t.Fatalf("unexpected SSE response: %s", body)
	}
}
