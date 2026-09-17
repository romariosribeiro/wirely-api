package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestActivityHistoryFilteringPagingAndDedupe(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Activity")
	if err != nil {
		t.Fatal(err)
	}
	events := []struct{ id, event, timestamp, messageID string }{
		{"evt_1", "message.received", "2026-09-16T10:00:00Z", "msg_1"},
		{"evt_2", "instance.status", "2026-09-16T11:00:00Z", ""},
		{"evt_3", "message.sent", "2026-09-16T12:00:00Z", "msg_2"},
		{"evt_4", "status.received", "2026-09-16T13:00:00Z", "status_1"},
	}
	for _, event := range events {
		inserted, err := store.SaveActivityEvent(ctx, event.id, instance.ID, event.event, event.timestamp, map[string]any{"id": event.messageID, "text": event.id})
		if err != nil || !inserted {
			t.Fatalf("save %s: inserted=%v err=%v", event.id, inserted, err)
		}
	}
	inserted, err := store.SaveActivityEvent(ctx, "evt_duplicate", instance.ID, "message.sent", "2026-09-16T14:00:00Z", map[string]any{"id": "msg_2"})
	if err != nil || inserted {
		t.Fatal("duplicate WhatsApp message ID must be ignored")
	}
	items, total, err := store.ListActivityEvents(ctx, instance.ID, "all", 1, 2)
	if err != nil || total != 4 || len(items) != 2 || items[0].ID != "evt_4" || items[1].ID != "evt_3" {
		t.Fatalf("unexpected page: total=%d items=%#v err=%v", total, items, err)
	}
	messages, total, err := store.ListActivityEvents(ctx, instance.ID, "messages", 1, 25)
	if err != nil || total != 2 || len(messages) != 2 {
		t.Fatalf("unexpected message filter: %d %#v %v", total, messages, err)
	}
	connections, total, err := store.ListActivityEvents(ctx, instance.ID, "connection", 1, 25)
	if err != nil || total != 1 || connections[0].Event != "instance.status" {
		t.Fatal("connection filter failed")
	}
	if _, _, err := store.ListActivityEvents(ctx, instance.ID, "bad", 1, 25); !errors.Is(err, ErrActivityFilter) {
		t.Fatal("invalid filter must fail")
	}
	loaded, err := store.GetActivityEvent(ctx, instance.ID, "evt_1")
	if err != nil || loaded.Data["text"] != "evt_1" {
		t.Fatalf("event data did not round-trip: %#v %v", loaded, err)
	}
}

func TestWebhookDeliveryHistoryAndCascade(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Deliveries")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveActivityEvent(ctx, "evt_delivery", instance.ID, "message.sent", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"id": "message_delivery"}); err != nil {
		t.Fatal(err)
	}
	for _, record := range []DeliveryRecord{
		{EventID: "evt_delivery", InstanceID: instance.ID, Event: "message.sent", URL: "https://example.com/hook", Attempt: 1, Status: "failed", HTTPStatus: 500, Error: "endpoint returned HTTP 500", Duration: 20 * time.Millisecond},
		{EventID: "evt_delivery", InstanceID: instance.ID, Event: "message.sent", URL: "https://example.com/hook", Attempt: 2, Status: "delivered", HTTPStatus: 204, Duration: 15 * time.Millisecond, Manual: true},
	} {
		if err := store.RecordWebhookDelivery(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	failed, total, err := store.ListWebhookDeliveries(ctx, instance.ID, "failed", 1, 25)
	if err != nil || total != 1 || failed[0].HTTPStatus != 500 {
		t.Fatalf("failed filter: %#v %d %v", failed, total, err)
	}
	all, total, err := store.ListWebhookDeliveries(ctx, instance.ID, "all", 1, 1)
	if err != nil || total != 2 || len(all) != 1 || !all[0].Manual {
		t.Fatalf("delivery paging: %#v %d %v", all, total, err)
	}
	loaded, err := store.GetWebhookDelivery(ctx, instance.ID, all[0].ID)
	if err != nil || loaded.Status != "delivered" {
		t.Fatal("get delivery failed", err)
	}
	if _, err := store.GetWebhookDelivery(ctx, instance.ID, 99999); !errors.Is(err, ErrDeliveryNotFound) {
		t.Fatal("missing delivery must be reported")
	}
	if err := store.RecordWebhookDelivery(ctx, DeliveryRecord{Status: "pending"}); err == nil {
		t.Fatal("invalid delivery status accepted")
	}
	if deleted, err := store.DeleteInstance(ctx, instance.ID); err != nil || !deleted {
		t.Fatal(err)
	}
	if items, total, err := store.ListWebhookDeliveries(ctx, instance.ID, "all", 1, 25); err != nil || total != 0 || len(items) != 0 {
		t.Fatal("instance deletion must cascade logs")
	}
}
