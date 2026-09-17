package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWebhookJobLifecycleAndIdempotency(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	instance, err := store.CreateInstance(context.Background(), "Webhook jobs")
	if err != nil {
		t.Fatal(err)
	}
	job := WebhookJob{EventID: "evt-job", InstanceID: instance.ID, Event: "message.received", PayloadJSON: `{"id":"evt-job"}`}
	inserted, err := store.EnqueueWebhookJob(context.Background(), job, false)
	if err != nil || !inserted {
		t.Fatalf("enqueue failed: inserted=%v err=%v", inserted, err)
	}
	inserted, err = store.EnqueueWebhookJob(context.Background(), job, false)
	if err != nil || inserted {
		t.Fatalf("duplicate was not ignored: inserted=%v err=%v", inserted, err)
	}
	claimed, err := store.ClaimWebhookJob(context.Background(), time.Now().Add(time.Second))
	if err != nil || claimed.Status != "processing" || claimed.Attempt != 1 || claimed.MaxAttempts != 5 {
		t.Fatalf("unexpected claimed job: %#v err=%v", claimed, err)
	}
	next := time.Now().UTC().Add(time.Minute)
	if err := store.FailWebhookJob(context.Background(), job.EventID, 429, "rate limited", next, false); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetWebhookJob(context.Background(), instance.ID, job.EventID)
	if err != nil || stored.Status != "retrying" || stored.LastHTTPCode != 429 || stored.LastError != "rate limited" {
		t.Fatalf("unexpected retry state: %#v err=%v", stored, err)
	}
	if _, err := store.ClaimWebhookJob(context.Background(), time.Now()); !errors.Is(err, ErrWebhookJobNotFound) {
		t.Fatalf("job claimed before due time: %v", err)
	}
	claimed, err = store.ClaimWebhookJob(context.Background(), next.Add(time.Second))
	if err != nil || claimed.Attempt != 2 {
		t.Fatalf("retry claim failed: %#v err=%v", claimed, err)
	}
	if err := store.CompleteWebhookJob(context.Background(), job.EventID, 204); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.GetWebhookJob(context.Background(), instance.ID, job.EventID)
	if stored.Status != "delivered" || stored.LastHTTPCode != 204 {
		t.Fatalf("unexpected delivered state: %#v", stored)
	}
	job.Manual = true
	inserted, err = store.EnqueueWebhookJob(context.Background(), job, true)
	if err != nil || !inserted {
		t.Fatalf("force enqueue failed: inserted=%v err=%v", inserted, err)
	}
	stored, _ = store.GetWebhookJob(context.Background(), instance.ID, job.EventID)
	if stored.Status != "queued" || stored.Attempt != 0 || !stored.Manual {
		t.Fatalf("unexpected forced state: %#v", stored)
	}
}

func TestWebhookJobRecoveryAndInstanceIsolation(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, _ := store.CreateInstance(context.Background(), "First")
	second, _ := store.CreateInstance(context.Background(), "Second")
	job := WebhookJob{EventID: "evt-recover", InstanceID: first.ID, Event: "instance.status", PayloadJSON: `{"id":"evt-recover"}`}
	if _, err := store.EnqueueWebhookJob(context.Background(), job, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimWebhookJob(context.Background(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecoverWebhookJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetWebhookJob(context.Background(), first.ID, job.EventID)
	if err != nil || stored.Status != "retrying" {
		t.Fatalf("job was not recovered: %#v err=%v", stored, err)
	}
	if _, err := store.GetWebhookJob(context.Background(), second.ID, job.EventID); !errors.Is(err, ErrWebhookJobNotFound) {
		t.Fatalf("job leaked to another instance: %v", err)
	}
}
