package webhook

import (
	"context"
	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeliveryHonorsPreferencesAndStopsRetryWhenDisabled(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Delivery")
	if err != nil {
		t.Fatal(err)
	}
	var deliveries atomic.Int32
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveries.Add(1)
		config, err := store.GetWebhookConfig(ctx, instance.ID)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if _, err := store.SaveWebhook(ctx, instance.ID, config.URL, false, config.Events, false); err != nil {
			t.Error(err)
		}
		w.WriteHeader(500)
	}))
	defer receiver.Close()
	if _, err := store.SaveWebhook(ctx, instance.ID, receiver.URL, true, []string{"messages"}, false); err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(store)
	dispatcher.retryDelay = func(int) time.Duration { return 0 }
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Close()
	dispatcher.Dispatch(engine.Event{ID: "status", InstanceID: instance.ID, Event: "instance.status"})
	time.Sleep(50 * time.Millisecond)
	if deliveries.Load() != 0 {
		t.Fatal("unselected event was delivered")
	}
	dispatcher.Dispatch(engine.Event{ID: "message", InstanceID: instance.ID, Event: "message.received"})
	waitFor(t, func() bool { return deliveries.Load() == 1 })
	time.Sleep(50 * time.Millisecond)
	if deliveries.Load() != 1 {
		t.Fatal("disabled webhook was retried")
	}
	dispatcher.Dispatch(engine.Event{ID: "another", InstanceID: instance.ID, Event: "message.sent"})
	if deliveries.Load() != 1 {
		t.Fatal("disabled webhook was called again")
	}
}
