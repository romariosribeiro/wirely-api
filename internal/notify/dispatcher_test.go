package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type fakeAlertStore struct {
	targets  storage.AlertTargets
	instance storage.Instance
}

func (f fakeAlertStore) AlertTargets(context.Context) (storage.AlertTargets, error) {
	return f.targets, nil
}
func (f fakeAlertStore) GetInstance(context.Context, string) (storage.Instance, error) {
	return f.instance, nil
}

func TestDispatcherOnlyRecoversAfterAnUnhealthyState(t *testing.T) {
	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { delivered.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	store := fakeAlertStore{instance: storage.Instance{ID: "one", Name: "Principal"}, targets: storage.AlertTargets{
		Telegram: storage.TelegramAlertConfig{Enabled: true, ChatID: "123", InstanceIDs: []string{"one"}}, Token: "secret",
	}}
	dispatcher := NewDispatcher(store)
	dispatcher.telegramURL = func(string) string { return server.URL }
	dispatcher.process(engine.Event{Event: "instance.status", InstanceID: "one", Data: map[string]any{"status": "connected"}})
	if delivered.Load() != 0 {
		t.Fatal("startup connection must not emit a recovery alert")
	}
	dispatcher.process(engine.Event{Event: "instance.status", InstanceID: "one", Data: map[string]any{"status": "disconnected"}})
	dispatcher.process(engine.Event{Event: "instance.status", InstanceID: "one", Data: map[string]any{"status": "connected"}})
	if delivered.Load() != 2 {
		t.Fatalf("expected outage and recovery alerts, got %d", delivered.Load())
	}
}

func TestBuildEmailStripsHeaderNewlines(t *testing.T) {
	message := string(buildEmail("sender@example.com\r\nBcc: stolen@example.com", "owner@example.com", "Alert\nBcc: stolen@example.com", "body"))
	if len(message) == 0 || message[:5] != "From:" {
		t.Fatal("invalid email")
	}
	if contains([]string{"one"}, "two") {
		t.Fatal("contains returned a false positive")
	}
}
