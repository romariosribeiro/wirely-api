package stream

import (
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func TestHubIsolatesInstances(t *testing.T) {
	hub := New()
	events, cancel := hub.Subscribe("one")
	defer cancel()
	hub.Publish(engine.Event{ID: "ignored", InstanceID: "two"})
	hub.Publish(engine.Event{ID: "received", InstanceID: "one"})
	select {
	case event := <-events:
		if event.ID != "received" {
			t.Fatalf("unexpected event %q", event.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not published")
	}
}
