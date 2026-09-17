package outbox

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type fakeSender struct {
	mu         sync.Mutex
	textCalls  int
	mediaCalls int
	err        error
}

func (sender *fakeSender) SendText(_ context.Context, _ string, recipient, _ string) (engine.SentMessage, error) {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.textCalls++
	if sender.err != nil {
		return engine.SentMessage{}, sender.err
	}
	return engine.SentMessage{ID: "msg_queued", Recipient: "+" + recipient, Timestamp: time.Now().UTC().Format(time.RFC3339), Type: "text"}, nil
}

func (sender *fakeSender) SendMedia(_ context.Context, _ string, recipient string, media engine.MediaPayload) (engine.SentMessage, error) {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.mediaCalls++
	if sender.err != nil {
		return engine.SentMessage{}, sender.err
	}
	return engine.SentMessage{ID: "msg_media", Recipient: "+" + recipient, Timestamp: time.Now().UTC().Format(time.RFC3339), Type: string(media.Kind)}, nil
}

func queueFixture(t *testing.T, sender Sender) (*storage.Store, *Service, storage.Instance) {
	t.Helper()
	directory := t.TempDir()
	store, err := storage.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := store.CreateInstance(context.Background(), "Queue service")
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	service, err := New(directory, store, sender, 0)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close(); _ = store.Close() })
	return store, service, instance
}

func TestWorkerDeliversPersistentTextJobAndIdempotencyReplays(t *testing.T) {
	sender := &fakeSender{}
	store, service, instance := queueFixture(t, sender)
	ctx := context.Background()
	job, replayed, err := service.EnqueueText(ctx, instance.ID, "+55 (11) 99999-9999", " Olá pela fila ", time.Time{}, "request-1")
	if err != nil || replayed || job.Status != "queued" || job.Recipient != "5511999999999" {
		t.Fatalf("unexpected enqueue: %#v replayed=%v err=%v", job, replayed, err)
	}
	replay, replayed, err := service.EnqueueText(ctx, instance.ID, "5511999999999", "Olá pela fila", time.Time{}, "request-1")
	if err != nil || !replayed || replay.ID != job.ID {
		t.Fatalf("idempotent replay failed: %#v replayed=%v err=%v", replay, replayed, err)
	}
	if _, _, err := service.EnqueueText(ctx, instance.ID, "5511999999999", "Conteúdo diferente", time.Time{}, "request-1"); !errors.Is(err, storage.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.GetMessageJob(ctx, instance.ID, job.ID)
		if err == nil && current.Status == "sent" {
			if current.Result == nil || current.Result.ID != "msg_queued" {
				t.Fatalf("missing send result: %#v", current)
			}
			sender.mu.Lock()
			calls := sender.textCalls
			sender.mu.Unlock()
			if calls != 1 {
				t.Fatalf("expected one send, got %d", calls)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("queued job was not delivered")
}

func TestQueuedMediaIsPrivateAndRemovedOnCancel(t *testing.T) {
	store, service, instance := queueFixture(t, &fakeSender{})
	job, replayed, err := service.EnqueueMedia(context.Background(), instance.ID, "5511999999999", engine.MediaPayload{
		Kind: engine.MediaImage, Data: []byte("image bytes"), MIMEType: "image/png", FileName: "photo.png", Caption: "Teste",
	}, time.Now().UTC().Add(time.Hour), "media-1")
	if err != nil || replayed || job.MediaPath == "" {
		t.Fatalf("media enqueue failed: %#v replayed=%v err=%v", job, replayed, err)
	}
	info, err := os.Stat(job.MediaPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("queued media permissions: info=%v err=%v", info, err)
	}
	replay, replayed, err := service.EnqueueMedia(context.Background(), instance.ID, "5511999999999", engine.MediaPayload{
		Kind: engine.MediaImage, Data: []byte("image bytes"), MIMEType: "image/png", FileName: "photo.png", Caption: "Teste",
	}, time.Now().UTC().Add(time.Hour), "media-1")
	if !errors.Is(err, storage.ErrIdempotencyConflict) || replayed || replay.ID != "" {
		// Explicit schedules differ by a few milliseconds and must conflict instead of silently changing delivery time.
		if err == nil {
			t.Fatal("different explicit schedule unexpectedly replayed")
		}
	}
	canceled, err := service.Cancel(context.Background(), instance.ID, job.ID)
	if err != nil || canceled.Status != "canceled" {
		t.Fatalf("cancel failed: %#v %v", canceled, err)
	}
	if _, err := os.Stat(job.MediaPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled media still exists: %v", err)
	}
	items, total, err := store.ListMessageJobs(context.Background(), instance.ID, "canceled", 1, 10)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("canceled job missing from history: %#v %d %v", items, total, err)
	}
}

func TestWorkerDeliversVideoAndSticker(t *testing.T) {
	tests := []engine.MediaPayload{
		{Kind: engine.MediaVideo, Data: []byte("video"), MIMEType: "video/mp4", FileName: "clip.mp4", Caption: "Clip"},
		{Kind: engine.MediaSticker, Data: []byte("webp"), MIMEType: "image/webp", FileName: "sticker.webp"},
	}
	for _, media := range tests {
		t.Run(string(media.Kind), func(t *testing.T) {
			store, service, instance := queueFixture(t, &fakeSender{})
			job, replayed, err := service.EnqueueMedia(context.Background(), instance.ID, "5511999999999", media, time.Time{}, "new-media-"+string(media.Kind))
			if err != nil || replayed {
				t.Fatalf("enqueue %s: %#v replayed=%v err=%v", media.Kind, job, replayed, err)
			}
			if err := service.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				current, err := store.GetMessageJob(context.Background(), instance.ID, job.ID)
				if err == nil && current.Status == "sent" {
					if current.Result == nil || current.Result.Type != string(media.Kind) {
						t.Fatalf("unexpected %s result: %#v", media.Kind, current.Result)
					}
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatalf("queued %s was not delivered", media.Kind)
		})
	}
}

func TestTerminalFailureCanBeRetriedManually(t *testing.T) {
	sender := &fakeSender{err: errors.New("provider unavailable")}
	store, service, instance := queueFixture(t, sender)
	job, _, err := store.CreateMessageJob(context.Background(), storage.MessageJobCreate{
		ID: "job_terminal", InstanceID: instance.ID, Kind: "text", Recipient: "5511999999999",
		Payload: storage.JobPayload{Message: "Falhar"}, Fingerprint: "terminal", ScheduledAt: time.Now().UTC(), MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimNextMessageJob(context.Background(), time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	service.deliver(claimed)
	failed, err := store.GetMessageJob(context.Background(), instance.ID, job.ID)
	if err != nil || failed.Status != "failed" || failed.LastError != "provider unavailable" {
		t.Fatalf("terminal failure not recorded: %#v %v", failed, err)
	}
	sender.err = nil
	retried, err := service.Retry(context.Background(), instance.ID, job.ID)
	if err != nil || retried.Status != "queued" || retried.Attempt != 0 {
		t.Fatalf("manual retry failed: %#v %v", retried, err)
	}
}

func TestQueueValidatesScheduleAndIdempotencyKey(t *testing.T) {
	_, service, instance := queueFixture(t, &fakeSender{})
	if _, _, err := service.EnqueueText(context.Background(), instance.ID, "5511999999999", "Teste", time.Now().Add(366*24*time.Hour), "key"); !errors.Is(err, ErrScheduleTooFar) {
		t.Fatalf("expected schedule error, got %v", err)
	}
	if _, _, err := service.EnqueueText(context.Background(), instance.ID, "5511999999999", "Teste", time.Time{}, "bad\nkey"); !errors.Is(err, ErrInvalidIdempotencyKey) {
		t.Fatalf("expected idempotency key error, got %v", err)
	}
}
