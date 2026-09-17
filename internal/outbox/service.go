package outbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/security"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

const (
	defaultWorkers     = 4
	defaultMaxAttempts = 5
	jobRetention       = 30 * 24 * time.Hour
)

var (
	ErrInvalidIdempotencyKey = errors.New("idempotency key must contain 1 to 128 printable characters")
	ErrScheduleTooFar        = errors.New("scheduled time must be within the next 365 days")
)

type Sender interface {
	SendText(context.Context, string, string, string) (engine.SentMessage, error)
	SendMedia(context.Context, string, string, engine.MediaPayload) (engine.SentMessage, error)
}

type Service struct {
	store     *storage.Store
	sender    Sender
	mediaDir  string
	rate      time.Duration
	workers   int
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	closeOnce sync.Once
	rateMu    sync.Mutex
	nextSlot  map[string]time.Time
}

func New(dataDirectory string, store *storage.Store, sender Sender, rate time.Duration) (*Service, error) {
	mediaDir := filepath.Join(dataDirectory, "outbox")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return nil, fmt.Errorf("create outbox directory: %w", err)
	}
	if rate < 0 {
		rate = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{store: store, sender: sender, mediaDir: mediaDir, rate: rate, workers: defaultWorkers,
		ctx: ctx, cancel: cancel, nextSlot: make(map[string]time.Time)}, nil
}

func (s *Service) Start() error {
	var startErr error
	s.startOnce.Do(func() {
		if err := s.store.RecoverMessageJobs(context.Background()); err != nil {
			startErr = fmt.Errorf("recover message jobs: %w", err)
			return
		}
		paths, err := s.store.PruneMessageJobs(context.Background(), jobRetention)
		if err != nil {
			startErr = fmt.Errorf("prune message jobs: %w", err)
			return
		}
		removeFiles(paths)
		for index := 0; index < s.workers; index++ {
			s.wg.Add(1)
			go s.worker()
		}
		s.wg.Add(1)
		go s.pruner()
	})
	return startErr
}

func (s *Service) Close() {
	s.closeOnce.Do(func() { s.cancel(); s.wg.Wait() })
}

func (s *Service) EnqueueText(ctx context.Context, instanceID, recipient, message string, scheduledAt time.Time, idempotencyKey string) (storage.MessageJob, bool, error) {
	phone, err := engine.NormalizeRecipient(recipient)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	message, err = engine.ValidateText(message)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	scheduleFingerprint := explicitSchedule(scheduledAt)
	scheduledAt, err = validateSchedule(scheduledAt)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	idempotencyKey, err = validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	payload := storage.JobPayload{Message: message}
	return s.create(ctx, storage.MessageJobCreate{InstanceID: instanceID, Kind: "text", Recipient: phone,
		Payload: payload, ScheduledAt: scheduledAt, IdempotencySchedule: scheduleFingerprint, MaxAttempts: defaultMaxAttempts, IdempotencyKey: idempotencyKey}, nil)
}

func (s *Service) EnqueueMedia(ctx context.Context, instanceID, recipient string, media engine.MediaPayload, scheduledAt time.Time, idempotencyKey string) (storage.MessageJob, bool, error) {
	phone, err := engine.NormalizeRecipient(recipient)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	if err := engine.ValidateMedia(&media); err != nil {
		return storage.MessageJob{}, false, err
	}
	scheduleFingerprint := explicitSchedule(scheduledAt)
	scheduledAt, err = validateSchedule(scheduledAt)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	idempotencyKey, err = validateIdempotencyKey(idempotencyKey)
	if err != nil {
		return storage.MessageJob{}, false, err
	}
	payload := storage.JobPayload{Caption: media.Caption, Voice: media.Voice, MIMEType: media.MIMEType, FileName: media.FileName, FileSize: len(media.Data)}
	return s.create(ctx, storage.MessageJobCreate{InstanceID: instanceID, Kind: string(media.Kind), Recipient: phone,
		Payload: payload, ScheduledAt: scheduledAt, IdempotencySchedule: scheduleFingerprint, MaxAttempts: defaultMaxAttempts, IdempotencyKey: idempotencyKey}, media.Data)
}

func (s *Service) create(ctx context.Context, input storage.MessageJobCreate, media []byte) (storage.MessageJob, bool, error) {
	identifier, err := security.RandomToken(18)
	if err != nil {
		return storage.MessageJob{}, false, fmt.Errorf("generate message job identifier: %w", err)
	}
	input.ID = "job_" + identifier
	if len(media) > 0 {
		input.MediaPath = filepath.Join(s.mediaDir, input.ID+".bin")
		if err := os.WriteFile(input.MediaPath, media, 0o600); err != nil {
			return storage.MessageJob{}, false, fmt.Errorf("store queued media: %w", err)
		}
	}
	input.Fingerprint, err = fingerprint(input, media)
	if err != nil {
		_ = os.Remove(input.MediaPath)
		return storage.MessageJob{}, false, err
	}
	job, replayed, err := s.store.CreateMessageJob(ctx, input)
	if err != nil || replayed {
		_ = os.Remove(input.MediaPath)
	}
	return job, replayed, err
}

func fingerprint(input storage.MessageJobCreate, media []byte) (string, error) {
	mediaDigest := sha256.Sum256(media)
	value := struct {
		InstanceID, Kind, Recipient, MediaSHA, ScheduledAt string
		Payload                                            storage.JobPayload
	}{input.InstanceID, input.Kind, input.Recipient, hex.EncodeToString(mediaDigest[:]), input.IdempotencySchedule, input.Payload}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func explicitSchedule(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func validateSchedule(value time.Time) (time.Time, error) {
	now := time.Now().UTC()
	if value.IsZero() || value.Before(now) {
		return now, nil
	}
	if value.After(now.Add(365 * 24 * time.Hour)) {
		return time.Time{}, ErrScheduleTooFar
	}
	return value.UTC(), nil
}

func validateIdempotencyKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > 128 {
		return "", ErrInvalidIdempotencyKey
	}
	for _, character := range value {
		if unicode.IsControl(character) || !unicode.IsPrint(character) {
			return "", ErrInvalidIdempotencyKey
		}
	}
	return value, nil
}

func (s *Service) Cancel(ctx context.Context, instanceID, id string) (storage.MessageJob, error) {
	job, err := s.store.CancelMessageJob(ctx, instanceID, id)
	if err == nil && job.MediaPath != "" {
		_ = os.Remove(job.MediaPath)
	}
	return job, err
}

func (s *Service) Retry(ctx context.Context, instanceID, id string) (storage.MessageJob, error) {
	job, err := s.store.GetMessageJob(ctx, instanceID, id)
	if err != nil {
		return storage.MessageJob{}, err
	}
	if job.MediaPath != "" {
		if _, err := os.Stat(job.MediaPath); err != nil {
			return storage.MessageJob{}, errors.New("queued media is no longer available")
		}
	}
	return s.store.RetryMessageJob(ctx, instanceID, id)
}

func (s *Service) PurgeInstance(ctx context.Context, instanceID string) error {
	paths, err := s.store.MessageJobMediaPaths(ctx, instanceID)
	if err != nil {
		return err
	}
	removeFiles(paths)
	return nil
}

func (s *Service) worker() {
	defer s.wg.Done()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		}
		job, err := s.store.ClaimNextMessageJob(s.ctx, time.Now().UTC())
		if errors.Is(err, storage.ErrJobNotFound) {
			timer.Reset(400 * time.Millisecond)
			continue
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("message queue claim failed", "error", err)
			}
			timer.Reset(time.Second)
			continue
		}
		if !s.waitRate(job.InstanceID) {
			return
		}
		s.deliver(job)
		timer.Reset(0)
	}
}

func (s *Service) waitRate(instanceID string) bool {
	now := time.Now()
	s.rateMu.Lock()
	slot := s.nextSlot[instanceID]
	if slot.Before(now) {
		slot = now
	}
	s.nextSlot[instanceID] = slot.Add(s.rate)
	s.rateMu.Unlock()
	if wait := time.Until(slot); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-s.ctx.Done():
			return false
		case <-timer.C:
		}
	}
	return true
}

func (s *Service) deliver(job storage.MessageJob) {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	var sent engine.SentMessage
	var err error
	switch job.Kind {
	case "text":
		sent, err = s.sender.SendText(ctx, job.InstanceID, job.Recipient, job.Payload.Message)
	case "image", "video", "audio", "document", "sticker":
		var data []byte
		data, err = os.ReadFile(job.MediaPath)
		if err == nil {
			sent, err = s.sender.SendMedia(ctx, job.InstanceID, job.Recipient, engine.MediaPayload{
				Kind: engine.MediaKind(job.Kind), Data: data, MIMEType: job.Payload.MIMEType,
				FileName: job.Payload.FileName, Caption: job.Payload.Caption, Voice: job.Payload.Voice,
			})
		}
	default:
		err = errors.New("unsupported queued message type")
	}
	if err == nil {
		result := storage.JobResult{ID: sent.ID, Recipient: sent.Recipient, Timestamp: sent.Timestamp, Type: sent.Type}
		if completeErr := s.store.CompleteMessageJob(context.Background(), job.ID, result); completeErr != nil {
			slog.Error("message queue completion failed", "job_id", job.ID, "error", completeErr)
			return
		}
		if job.MediaPath != "" {
			_ = os.Remove(job.MediaPath)
		}
		return
	}
	terminal := job.Attempt >= job.MaxAttempts || !retryable(err)
	nextAttempt := time.Now().UTC().Add(retryDelay(job.Attempt))
	if failErr := s.store.FailMessageJob(context.Background(), job.ID, err.Error(), nextAttempt, terminal); failErr != nil {
		slog.Error("message queue failure update failed", "job_id", job.ID, "error", failErr)
	}
}

func retryable(err error) bool {
	return err != nil && !errors.Is(err, os.ErrNotExist)
}

func retryDelay(attempt int) time.Duration {
	delays := []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute}
	if attempt <= 0 {
		return delays[0]
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}

func (s *Service) pruner() {
	defer s.wg.Done()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			paths, err := s.store.PruneMessageJobs(s.ctx, jobRetention)
			if err != nil {
				slog.Error("message queue prune failed", "error", err)
				continue
			}
			removeFiles(paths)
		}
	}
}

func removeFiles(paths []string) {
	for _, path := range paths {
		if path != "" {
			_ = os.Remove(path)
		}
	}
}
