package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMessageJobLifecycleAndIdempotency(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Queue")
	if err != nil {
		t.Fatal(err)
	}
	input := MessageJobCreate{
		ID: "job_one", InstanceID: instance.ID, Kind: "text", Recipient: "5511999999999",
		Payload: JobPayload{Message: "Olá"}, Fingerprint: "fingerprint-one", IdempotencyKey: "order-123",
		ScheduledAt: time.Now().UTC().Add(-time.Minute), MaxAttempts: 3,
	}
	created, replayed, err := store.CreateMessageJob(ctx, input)
	if err != nil || replayed || created.Status != "queued" || created.Payload.Message != "Olá" || created.MaxAttempts != 3 {
		t.Fatalf("unexpected create result: %#v replayed=%v err=%v", created, replayed, err)
	}
	replayedJob, replayed, err := store.CreateMessageJob(ctx, MessageJobCreate{
		ID: "job_duplicate", InstanceID: instance.ID, Kind: "text", Recipient: input.Recipient,
		Payload: input.Payload, Fingerprint: input.Fingerprint, IdempotencyKey: input.IdempotencyKey,
	})
	if err != nil || !replayed || replayedJob.ID != created.ID {
		t.Fatalf("idempotent replay failed: %#v replayed=%v err=%v", replayedJob, replayed, err)
	}
	_, _, err = store.CreateMessageJob(ctx, MessageJobCreate{
		ID: "job_conflict", InstanceID: instance.ID, Kind: "text", Recipient: input.Recipient,
		Payload: JobPayload{Message: "Outro"}, Fingerprint: "different", IdempotencyKey: input.IdempotencyKey,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	claimed, err := store.ClaimNextMessageJob(ctx, time.Now().UTC().Add(time.Second))
	if err != nil || claimed.ID != created.ID || claimed.Status != "processing" || claimed.Attempt != 1 {
		t.Fatalf("unexpected claim: %#v err=%v", claimed, err)
	}
	if err := store.FailMessageJob(ctx, claimed.ID, "temporary", time.Now().UTC().Add(-time.Second), false); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimNextMessageJob(ctx, time.Now().UTC())
	if err != nil || claimed.Attempt != 2 {
		t.Fatalf("retry claim failed: %#v err=%v", claimed, err)
	}
	result := JobResult{ID: "msg_one", Recipient: "+5511999999999", Timestamp: time.Now().UTC().Format(time.RFC3339), Type: "text"}
	if err := store.CompleteMessageJob(ctx, claimed.ID, result); err != nil {
		t.Fatal(err)
	}
	completed, err := store.GetMessageJob(ctx, instance.ID, claimed.ID)
	if err != nil || completed.Status != "sent" || completed.Result == nil || completed.Result.ID != "msg_one" || completed.CompletedAt == "" {
		t.Fatalf("completion failed: %#v err=%v", completed, err)
	}
	if _, err := store.ClaimNextMessageJob(ctx, time.Now().UTC()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected empty queue, got %v", err)
	}
	items, total, err := store.ListMessageJobs(ctx, instance.ID, "sent", 1, 25)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("unexpected job list: %#v total=%d err=%v", items, total, err)
	}
}

func TestMessageJobCancelRetryRecoveryAndCascade(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Queue states")
	if err != nil {
		t.Fatal(err)
	}
	create := func(id string) MessageJob {
		job, _, err := store.CreateMessageJob(ctx, MessageJobCreate{ID: id, InstanceID: instance.ID, Kind: "image", Recipient: "5511999999999",
			Payload: JobPayload{FileName: id + ".png"}, MediaPath: "/tmp/" + id, Fingerprint: id, ScheduledAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	queued := create("job_cancel")
	canceled, err := store.CancelMessageJob(ctx, instance.ID, queued.ID)
	if err != nil || canceled.Status != "canceled" {
		t.Fatalf("cancel failed: %#v %v", canceled, err)
	}
	if _, err := store.RetryMessageJob(ctx, instance.ID, queued.ID); !errors.Is(err, ErrJobStateConflict) {
		t.Fatalf("canceled job must not retry: %v", err)
	}
	failed := create("job_failed")
	claimed, err := store.ClaimNextMessageJob(ctx, time.Now().UTC().Add(time.Second))
	if err != nil || claimed.ID != failed.ID {
		t.Fatalf("claim failed job setup: %#v %v", claimed, err)
	}
	if err := store.FailMessageJob(ctx, failed.ID, "permanent", time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	retried, err := store.RetryMessageJob(ctx, instance.ID, failed.ID)
	if err != nil || retried.Status != "queued" || retried.Attempt != 0 {
		t.Fatalf("manual retry failed: %#v %v", retried, err)
	}
	processing, err := store.ClaimNextMessageJob(ctx, time.Now().UTC().Add(time.Second))
	if err != nil || processing.ID != failed.ID {
		t.Fatal("reclaimed job failed", err)
	}
	if err := store.RecoverMessageJobs(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.GetMessageJob(ctx, instance.ID, processing.ID)
	if err != nil || recovered.Status != "retrying" {
		t.Fatalf("recovery failed: %#v %v", recovered, err)
	}
	paths, err := store.MessageJobMediaPaths(ctx, instance.ID)
	if err != nil || len(paths) != 2 {
		t.Fatalf("media paths failed: %#v %v", paths, err)
	}
	if deleted, err := store.DeleteInstance(ctx, instance.ID); err != nil || !deleted {
		t.Fatal(err)
	}
	if _, err := store.GetMessageJob(ctx, instance.ID, processing.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("instance cascade failed: %v", err)
	}
}

func TestPruneMessageJobsRemovesOnlyExpiredTerminalJobs(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Queue retention")
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := store.CreateMessageJob(ctx, MessageJobCreate{
		ID: "job_expired", InstanceID: instance.ID, Kind: "image", Recipient: "5511999999999",
		Payload: JobPayload{FileName: "expired.png"}, MediaPath: "/tmp/job-expired.bin", Fingerprint: "expired", ScheduledAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextMessageJob(ctx, time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteMessageJob(ctx, claimed.ID, JobResult{ID: "message-expired", Type: "image"}); err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().UTC().Add(-31 * 24 * time.Hour).UnixMilli()
	if _, err := store.db.ExecContext(ctx, "UPDATE message_jobs SET completed_at = ? WHERE id = ?", expiredAt, old.ID); err != nil {
		t.Fatal(err)
	}
	active, _, err := store.CreateMessageJob(ctx, MessageJobCreate{
		ID: "job_active", InstanceID: instance.ID, Kind: "text", Recipient: "5511999999999",
		Payload: JobPayload{Message: "Keep"}, Fingerprint: "active", ScheduledAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := store.PruneMessageJobs(ctx, 30*24*time.Hour)
	if err != nil || len(paths) != 1 || paths[0] != "/tmp/job-expired.bin" {
		t.Fatalf("unexpected prune result: paths=%#v err=%v", paths, err)
	}
	if _, err := store.GetMessageJob(ctx, instance.ID, old.ID); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expired terminal job was retained: %v", err)
	}
	if kept, err := store.GetMessageJob(ctx, instance.ID, active.ID); err != nil || kept.Status != "queued" {
		t.Fatalf("active job was pruned: %#v %v", kept, err)
	}
}
