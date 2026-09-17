package storage

import (
	"context"
	"testing"
	"time"
)

func TestMetricsSnapshotAggregatesRangeAndCurrentQueue(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	primary, err := store.CreateInstance(ctx, "Primary metrics")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateInstanceStatus(ctx, primary.ID, "connected"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateInstance(ctx, "Secondary metrics"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, event := range []struct {
		id, name string
		at       time.Time
	}{
		{"metric-sent", "message.sent", now.Add(-time.Hour)},
		{"metric-received", "message.received", now.Add(-2 * time.Hour)},
		{"metric-old", "message.sent", now.Add(-48 * time.Hour)},
	} {
		if _, err := store.SaveActivityEvent(ctx, event.id, primary.ID, event.name, event.at.Format(time.RFC3339Nano), map[string]any{"id": event.id, "chat": "5511999999999@s.whatsapp.net"}); err != nil {
			t.Fatal(err)
		}
	}

	createJob := func(id string, scheduled time.Time, maxAttempts int) MessageJob {
		job, _, err := store.CreateMessageJob(ctx, MessageJobCreate{ID: id, InstanceID: primary.ID, Kind: "text", Recipient: "5511999999999",
			Payload: JobPayload{Message: id}, Fingerprint: id, ScheduledAt: scheduled, MaxAttempts: maxAttempts})
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	sent := createJob("job_metric_sent", now, 5)
	claimed, err := store.ClaimNextMessageJob(ctx, now.Add(time.Second))
	if err != nil || claimed.ID != sent.ID {
		t.Fatalf("claim sent setup: %#v %v", claimed, err)
	}
	if err := store.CompleteMessageJob(ctx, sent.ID, JobResult{ID: "wa_metric_sent", Type: "text"}); err != nil {
		t.Fatal(err)
	}
	failed := createJob("job_metric_failed", now, 1)
	claimed, err = store.ClaimNextMessageJob(ctx, now.Add(time.Second))
	if err != nil || claimed.ID != failed.ID {
		t.Fatalf("claim failed setup: %#v %v", claimed, err)
	}
	if err := store.FailMessageJob(ctx, failed.ID, "provider failed", now, true); err != nil {
		t.Fatal(err)
	}
	createJob("job_metric_queued", now.Add(time.Hour), 5)

	for _, delivery := range []DeliveryRecord{
		{EventID: "metric-sent", InstanceID: primary.ID, Event: "message.sent", URL: "https://example.com", Attempt: 1, Status: "delivered", HTTPStatus: 204, Duration: 20 * time.Millisecond},
		{EventID: "metric-received", InstanceID: primary.ID, Event: "message.received", URL: "https://example.com", Attempt: 1, Status: "failed", HTTPStatus: 500, Duration: 40 * time.Millisecond},
	} {
		if err := store.RecordWebhookDelivery(ctx, delivery); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, err := store.Metrics(ctx, now.Add(-24*time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Instances.Total != 2 || snapshot.Instances.Connected != 1 {
		t.Fatalf("unexpected instance metrics: %#v", snapshot.Instances)
	}
	if snapshot.Messages.Sent != 1 || snapshot.Messages.Received != 1 || snapshot.Messages.Total != 2 {
		t.Fatalf("unexpected message metrics: %#v", snapshot.Messages)
	}
	if snapshot.Queue.Pending != 1 || snapshot.Queue.Queued != 1 || snapshot.Queue.Sent != 1 || snapshot.Queue.Failed != 1 {
		t.Fatalf("unexpected queue metrics: %#v", snapshot.Queue)
	}
	if snapshot.Webhooks.Delivered != 1 || snapshot.Webhooks.Failed != 1 || snapshot.Webhooks.SuccessRate != 50 || snapshot.Webhooks.AverageTimeMS != 30 {
		t.Fatalf("unexpected webhook metrics: %#v", snapshot.Webhooks)
	}
	if len(snapshot.ByInstance) != 2 || snapshot.ByInstance[0].Name != "Primary metrics" || snapshot.ByInstance[0].Messages.Total != 2 {
		t.Fatalf("unexpected per-instance metrics: %#v", snapshot.ByInstance)
	}
}

func TestMetricsRejectsInvalidRange(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if _, err := store.Metrics(context.Background(), now, now); err == nil {
		t.Fatal("invalid metrics range accepted")
	}
}
