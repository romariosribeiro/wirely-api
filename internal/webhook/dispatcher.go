package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/security"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

const (
	defaultAttempts = 5
	requestTimeout  = 10 * time.Second
	workerPoll      = time.Second
)

var ErrWebhookUnavailable = errors.New("webhook is disabled or does not accept this event")

type jobStore interface {
	GetWebhookTarget(context.Context, string) (storage.WebhookTarget, error)
	RecordWebhookDelivery(context.Context, storage.DeliveryRecord) error
	EnqueueWebhookJob(context.Context, storage.WebhookJob, bool) (bool, error)
	RecoverWebhookJobs(context.Context) error
	ClaimWebhookJob(context.Context, time.Time) (storage.WebhookJob, error)
	CompleteWebhookJob(context.Context, string, int) error
	FailWebhookJob(context.Context, string, int, string, time.Time, bool) error
}

type Dispatcher struct {
	store      jobStore
	client     *http.Client
	attempts   int
	retryDelay func(int) time.Duration
	now        func() time.Time
	wake       chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	startOnce  sync.Once
	closeOnce  sync.Once
}

func NewDispatcher(store jobStore) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{
		store: store, attempts: defaultAttempts, now: func() time.Time { return time.Now().UTC() },
		client:     &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		retryDelay: retryBackoff, wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel, done: make(chan struct{}),
	}
}

func (d *Dispatcher) Start() error {
	if err := d.store.RecoverWebhookJobs(context.Background()); err != nil {
		return err
	}
	d.startOnce.Do(func() { go d.run() })
	d.notify()
	return nil
}

func (d *Dispatcher) Close() {
	d.closeOnce.Do(func() {
		d.cancel()
		select {
		case <-d.done:
		case <-time.After(5 * time.Second):
		}
	})
}

func (d *Dispatcher) Dispatch(event engine.Event) {
	if _, err := d.enqueue(event, false, false); err != nil && !errors.Is(err, ErrWebhookUnavailable) {
		slog.Error("webhook enqueue failed", "event_id", event.ID, "instance_id", event.InstanceID, "error", err)
	}
}

func (d *Dispatcher) Retry(event engine.Event) error {
	_, err := d.enqueue(event, true, true)
	return err
}

func (d *Dispatcher) Test(instanceID string) (string, error) {
	token, err := security.RandomToken(18)
	if err != nil {
		return "", err
	}
	event := engine.Event{
		ID: "evt_test_" + token, Event: "webhook.test", InstanceID: instanceID,
		Timestamp: d.now().Format(time.RFC3339), Data: map[string]any{"message": "Wirely webhook test"},
	}
	_, err = d.enqueue(event, true, false)
	return event.ID, err
}

func (d *Dispatcher) enqueue(event engine.Event, manual, force bool) (bool, error) {
	target, err := d.store.GetWebhookTarget(context.Background(), event.InstanceID)
	if err != nil {
		return false, fmt.Errorf("webhook target lookup: %w", err)
	}
	if target.URL == "" || target.Secret == "" || !target.Allows(event.Event) {
		return false, ErrWebhookUnavailable
	}
	body, err := json.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("encode webhook event: %w", err)
	}
	inserted, err := d.store.EnqueueWebhookJob(context.Background(), storage.WebhookJob{
		EventID: event.ID, InstanceID: event.InstanceID, Event: event.Event,
		PayloadJSON: string(body), MaxAttempts: d.attempts, Manual: manual,
	}, force)
	if err != nil {
		return false, err
	}
	if inserted {
		d.notify()
	}
	return inserted, nil
}

func (d *Dispatcher) notify() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) run() {
	defer close(d.done)
	ticker := time.NewTicker(workerPoll)
	defer ticker.Stop()
	for {
		d.processDue()
		select {
		case <-d.ctx.Done():
			return
		case <-d.wake:
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) processDue() {
	for {
		job, err := d.store.ClaimWebhookJob(d.ctx, d.now())
		if errors.Is(err, storage.ErrWebhookJobNotFound) || errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			slog.Error("webhook job claim failed", "error", err)
			return
		}
		d.process(job)
	}
}

func (d *Dispatcher) process(job storage.WebhookJob) {
	var event engine.Event
	if err := json.Unmarshal([]byte(job.PayloadJSON), &event); err != nil {
		d.fail(job, 0, fmt.Errorf("decode webhook event: %w", err), 0, true)
		return
	}
	target, err := d.store.GetWebhookTarget(d.ctx, job.InstanceID)
	if err != nil {
		d.fail(job, 0, fmt.Errorf("refresh webhook target: %w", err), 0, job.Attempt >= job.MaxAttempts)
		return
	}
	if target.URL == "" || target.Secret == "" || !target.Allows(job.Event) {
		d.fail(job, 0, ErrWebhookUnavailable, 0, true)
		return
	}
	status, duration, retryAfter, deliveryErr := d.deliver(target, event, []byte(job.PayloadJSON), job.Attempt)
	d.record(event, target.URL, job.Attempt, status, duration, deliveryErr, job.Manual)
	if deliveryErr == nil {
		if err := d.store.CompleteWebhookJob(context.Background(), job.EventID, status); err != nil {
			slog.Error("webhook completion failed", "event_id", job.EventID, "error", err)
		}
		return
	}
	dead := job.Attempt >= job.MaxAttempts
	d.fail(job, status, deliveryErr, retryAfter, dead)
}

func (d *Dispatcher) fail(job storage.WebhookJob, status int, deliveryErr error, retryAfter time.Duration, dead bool) {
	delay := d.retryDelay(job.Attempt)
	if retryAfter > delay {
		delay = retryAfter
	}
	next := d.now().Add(delay)
	if err := d.store.FailWebhookJob(context.Background(), job.EventID, status, deliveryErr.Error(), next, dead); err != nil {
		slog.Error("webhook failure state could not be saved", "event_id", job.EventID, "error", err)
	}
	if dead {
		slog.Error("webhook delivery exhausted", "event_id", job.EventID, "instance_id", job.InstanceID, "attempts", job.Attempt, "error", deliveryErr)
	}
}

func (d *Dispatcher) record(event engine.Event, url string, attempt, httpStatus int, duration time.Duration, deliveryErr error, manual bool) {
	status, message := "delivered", ""
	if deliveryErr != nil {
		status, message = "failed", deliveryErr.Error()
	}
	err := d.store.RecordWebhookDelivery(context.Background(), storage.DeliveryRecord{
		EventID: event.ID, InstanceID: event.InstanceID, Event: event.Event, URL: url,
		Attempt: attempt, Status: status, HTTPStatus: httpStatus, Error: message,
		Duration: duration, Manual: manual,
	})
	if err != nil {
		slog.Error("webhook delivery log failed", "event_id", event.ID, "instance_id", event.InstanceID, "error", err)
	}
}

func (d *Dispatcher) deliver(target storage.WebhookTarget, event engine.Event, body []byte, attempt int) (int, time.Duration, time.Duration, error) {
	ctx, cancel := context.WithTimeout(d.ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Wirely-Webhook/1.0")
	request.Header.Set("X-Wirely-Event", event.Event)
	request.Header.Set("X-Wirely-Delivery", event.ID)
	request.Header.Set("X-Wirely-Attempt", strconv.Itoa(attempt))
	request.Header.Set("X-Wirely-Signature", signature(target.Secret, body))
	started := time.Now()
	response, err := d.client.Do(request)
	duration := time.Since(started)
	if err != nil {
		return 0, duration, 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, duration, parseRetryAfter(response.Header.Get("Retry-After"), d.now()), fmt.Errorf("endpoint returned HTTP %d", response.StatusCode)
	}
	return response.StatusCode, duration, 0, nil
}

func retryBackoff(attempt int) time.Duration {
	delays := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if attempt < 1 {
		return delays[0]
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return min(time.Duration(seconds)*time.Second, 24*time.Hour)
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return min(when.Sub(now), 24*time.Hour)
}

func signature(secret string, body []byte) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return "sha256=" + hex.EncodeToString(digest.Sum(nil))
}
