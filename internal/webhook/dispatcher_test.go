package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func webhookTestStore(t *testing.T, endpoint string) (*storage.Store, storage.Instance, string) {
	t.Helper()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	instance, err := store.CreateInstance(context.Background(), "Webhook queue")
	if err != nil {
		t.Fatal(err)
	}
	config, err := store.SaveWebhook(context.Background(), instance.ID, endpoint, true, []string{"messages"}, false)
	if err != nil {
		t.Fatal(err)
	}
	return store, instance, config.Secret
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not reached")
}

func TestDispatcherPersistsSignsRetriesAndDeduplicates(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	var body []byte
	var headers http.Header
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		mu.Lock()
		attempts++
		current := attempts
		body = append([]byte(nil), payload...)
		headers = r.Header.Clone()
		mu.Unlock()
		if current == 1 {
			http.Error(w, "retry", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()

	store, instance, secret := webhookTestStore(t, endpoint.URL)
	dispatcher := NewDispatcher(store)
	dispatcher.client = endpoint.Client()
	dispatcher.retryDelay = func(int) time.Duration { return 0 }
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Close()
	event := engine.Event{ID: "evt_test", Event: "message.received", InstanceID: instance.ID,
		Timestamp: "2026-09-16T20:00:00Z", Data: map[string]any{"text": "hello"}}
	dispatcher.Dispatch(event)
	waitFor(t, func() bool {
		job, err := store.GetWebhookJob(context.Background(), instance.ID, event.ID)
		return err == nil && job.Status == "delivered"
	})

	mu.Lock()
	if attempts != 2 {
		t.Fatalf("expected two delivery attempts, got %d", attempts)
	}
	if headers.Get("X-Wirely-Event") != event.Event || headers.Get("X-Wirely-Delivery") != event.ID || headers.Get("X-Wirely-Attempt") != "2" {
		t.Fatalf("unexpected webhook headers: %#v", headers)
	}
	if headers.Get("X-Wirely-Signature") != signature(secret, body) {
		t.Fatal("webhook signature does not match")
	}
	mu.Unlock()
	var delivered engine.Event
	if err := json.Unmarshal(body, &delivered); err != nil || delivered.InstanceID != instance.ID {
		t.Fatalf("unexpected payload: %#v %v", delivered, err)
	}

	dispatcher.Dispatch(event)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Fatalf("duplicate event was delivered again: %d", attempts)
	}
}

func TestDispatcherRecoversProcessingJobAndManualRetry(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()
	store, instance, _ := webhookTestStore(t, endpoint.URL)
	event := engine.Event{ID: "evt_recover", Event: "message.received", InstanceID: instance.ID, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	body, _ := json.Marshal(event)
	if _, err := store.EnqueueWebhookJob(context.Background(), storage.WebhookJob{
		EventID: event.ID, InstanceID: instance.ID, Event: event.Event, PayloadJSON: string(body), MaxAttempts: 5,
	}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimWebhookJob(context.Background(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	dispatcher := NewDispatcher(store)
	dispatcher.client = endpoint.Client()
	if err := dispatcher.Start(); err != nil {
		t.Fatal(err)
	}
	defer dispatcher.Close()
	waitFor(t, func() bool {
		job, err := store.GetWebhookJob(context.Background(), instance.ID, event.ID)
		return err == nil && job.Status == "delivered"
	})
	if err := dispatcher.Retry(event); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return requests.Load() == 2 })
	deliveries, total, err := store.ListWebhookDeliveries(context.Background(), instance.ID, "delivered", 1, 10)
	if err != nil || total != 2 || !deliveries[0].Manual {
		t.Fatalf("manual delivery missing: %#v %d %v", deliveries, total, err)
	}
}

func TestRetryAfterAndDisabledWebhook(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	if delay := parseRetryAfter("120", now); delay != 2*time.Minute {
		t.Fatalf("seconds Retry-After: %s", delay)
	}
	if delay := parseRetryAfter(now.Add(5*time.Minute).Format(http.TimeFormat), now); delay != 5*time.Minute {
		t.Fatalf("date Retry-After: %s", delay)
	}
	if delay := parseRetryAfter(strconv.Itoa(48*60*60), now); delay != 24*time.Hour {
		t.Fatalf("Retry-After cap: %s", delay)
	}
	if retryBackoff(1) != time.Minute || retryBackoff(2) != 5*time.Minute || retryBackoff(3) != 15*time.Minute || retryBackoff(4) != time.Hour {
		t.Fatal("unexpected retry schedule")
	}

	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	instance, err := store.CreateInstance(context.Background(), "Disabled webhook")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := NewDispatcher(store)
	event := engine.Event{ID: "evt_disabled", Event: "message.received", InstanceID: instance.ID}
	if err := dispatcher.Retry(event); !errors.Is(err, ErrWebhookUnavailable) {
		t.Fatalf("disabled webhook accepted: %v", err)
	}
}
