package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrWebhookJobNotFound = errors.New("webhook job not found")

type WebhookJob struct {
	EventID      string    `json:"eventId"`
	InstanceID   string    `json:"instanceId"`
	Event        string    `json:"event"`
	PayloadJSON  string    `json:"-"`
	Status       string    `json:"status"`
	Attempt      int       `json:"attempt"`
	MaxAttempts  int       `json:"maxAttempts"`
	NextAttempt  time.Time `json:"nextAttemptAt,omitempty"`
	LastHTTPCode int       `json:"lastHttpStatus,omitempty"`
	LastError    string    `json:"lastError,omitempty"`
	Manual       bool      `json:"manual"`
}

func (s *Store) EnqueueWebhookJob(ctx context.Context, job WebhookJob, force bool) (bool, error) {
	if job.EventID == "" || job.InstanceID == "" || job.Event == "" || job.PayloadJSON == "" {
		return false, errors.New("webhook job identity and payload are required")
	}
	if job.MaxAttempts < 1 {
		job.MaxAttempts = 5
	}
	now := time.Now().UTC().UnixMilli()
	manual := 0
	if job.Manual {
		manual = 1
	}
	var result sql.Result
	var err error
	if force {
		result, err = s.db.ExecContext(ctx, `
INSERT INTO webhook_jobs
(event_id, instance_id, event, payload_json, status, attempt, max_attempts, next_attempt_at, last_http_status, last_error, manual, created_at, updated_at)
VALUES (?, ?, ?, ?, 'queued', 0, ?, ?, 0, '', ?, ?, ?)
ON CONFLICT(event_id) DO UPDATE SET
instance_id = excluded.instance_id, event = excluded.event, payload_json = excluded.payload_json,
status = 'queued', attempt = 0, max_attempts = excluded.max_attempts,
next_attempt_at = excluded.next_attempt_at, last_http_status = 0, last_error = '',
manual = excluded.manual, updated_at = excluded.updated_at`,
			job.EventID, job.InstanceID, job.Event, job.PayloadJSON, job.MaxAttempts, now, manual, now, now)
	} else {
		result, err = s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO webhook_jobs
(event_id, instance_id, event, payload_json, status, attempt, max_attempts, next_attempt_at, last_http_status, last_error, manual, created_at, updated_at)
VALUES (?, ?, ?, ?, 'queued', 0, ?, ?, 0, '', ?, ?, ?)`,
			job.EventID, job.InstanceID, job.Event, job.PayloadJSON, job.MaxAttempts, now, manual, now, now)
	}
	if err != nil {
		return false, fmt.Errorf("enqueue webhook job: %w", err)
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) RecoverWebhookJobs(ctx context.Context) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE webhook_jobs
SET status = 'retrying', next_attempt_at = ?, updated_at = ?
WHERE status = 'processing'`, now, now)
	if err != nil {
		return fmt.Errorf("recover webhook jobs: %w", err)
	}
	return nil
}

func (s *Store) ClaimWebhookJob(ctx context.Context, now time.Time) (WebhookJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WebhookJob{}, err
	}
	defer tx.Rollback()
	var job WebhookJob
	var nextAttempt int64
	var manual int
	err = tx.QueryRowContext(ctx, `SELECT event_id, instance_id, event, payload_json, status, attempt, max_attempts,
next_attempt_at, last_http_status, last_error, manual
FROM webhook_jobs
WHERE status IN ('queued', 'retrying') AND next_attempt_at <= ?
ORDER BY next_attempt_at, created_at, event_id LIMIT 1`, now.UTC().UnixMilli()).Scan(
		&job.EventID, &job.InstanceID, &job.Event, &job.PayloadJSON, &job.Status, &job.Attempt,
		&job.MaxAttempts, &nextAttempt, &job.LastHTTPCode, &job.LastError, &manual)
	if errors.Is(err, sql.ErrNoRows) {
		return WebhookJob{}, ErrWebhookJobNotFound
	}
	if err != nil {
		return WebhookJob{}, fmt.Errorf("claim webhook job: %w", err)
	}
	job.Attempt++
	result, err := tx.ExecContext(ctx, `UPDATE webhook_jobs SET status = 'processing', attempt = ?, updated_at = ?
WHERE event_id = ? AND status IN ('queued', 'retrying')`, job.Attempt, now.UTC().UnixMilli(), job.EventID)
	if err != nil {
		return WebhookJob{}, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return WebhookJob{}, ErrWebhookJobNotFound
	}
	if err := tx.Commit(); err != nil {
		return WebhookJob{}, err
	}
	job.Status = "processing"
	job.NextAttempt = time.UnixMilli(nextAttempt).UTC()
	job.Manual = manual != 0
	return job, nil
}

func (s *Store) CompleteWebhookJob(ctx context.Context, eventID string, httpStatus int) error {
	result, err := s.db.ExecContext(ctx, `UPDATE webhook_jobs
SET status = 'delivered', last_http_status = ?, last_error = '', updated_at = ?
WHERE event_id = ? AND status = 'processing'`, httpStatus, time.Now().UTC().UnixMilli(), eventID)
	if err != nil {
		return fmt.Errorf("complete webhook job: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ErrWebhookJobNotFound
	}
	return nil
}

func (s *Store) FailWebhookJob(ctx context.Context, eventID string, httpStatus int, message string, next time.Time, dead bool) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	status := "retrying"
	if dead {
		status = "dead"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE webhook_jobs
SET status = ?, next_attempt_at = ?, last_http_status = ?, last_error = ?, updated_at = ?
WHERE event_id = ? AND status = 'processing'`, status, next.UTC().UnixMilli(), httpStatus, message,
		time.Now().UTC().UnixMilli(), eventID)
	if err != nil {
		return fmt.Errorf("fail webhook job: %w", err)
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return ErrWebhookJobNotFound
	}
	return nil
}

func (s *Store) GetWebhookJob(ctx context.Context, instanceID, eventID string) (WebhookJob, error) {
	var job WebhookJob
	var nextAttempt int64
	var manual int
	err := s.db.QueryRowContext(ctx, `SELECT event_id, instance_id, event, payload_json, status, attempt,
max_attempts, next_attempt_at, last_http_status, last_error, manual
FROM webhook_jobs WHERE instance_id = ? AND event_id = ?`, instanceID, eventID).Scan(
		&job.EventID, &job.InstanceID, &job.Event, &job.PayloadJSON, &job.Status, &job.Attempt,
		&job.MaxAttempts, &nextAttempt, &job.LastHTTPCode, &job.LastError, &manual)
	if errors.Is(err, sql.ErrNoRows) {
		return WebhookJob{}, ErrWebhookJobNotFound
	}
	if err != nil {
		return WebhookJob{}, err
	}
	job.NextAttempt = time.UnixMilli(nextAttempt).UTC()
	job.Manual = manual != 0
	return job, nil
}
