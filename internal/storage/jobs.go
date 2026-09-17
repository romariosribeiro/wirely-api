package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrJobNotFound         = errors.New("message job not found")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with different content")
	ErrJobStateConflict    = errors.New("message job cannot be changed in its current state")
	ErrMessageJobFilter    = errors.New("invalid message job filter")
)

type JobPayload struct {
	Message  string `json:"message,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Voice    bool   `json:"voice,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	FileName string `json:"fileName,omitempty"`
	FileSize int    `json:"fileSize,omitempty"`
}

type JobResult struct {
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
}

type MessageJob struct {
	ID             string     `json:"id"`
	InstanceID     string     `json:"instanceId"`
	Kind           string     `json:"kind"`
	Recipient      string     `json:"recipient"`
	Payload        JobPayload `json:"payload"`
	MediaPath      string     `json:"-"`
	Fingerprint    string     `json:"-"`
	IdempotencyKey string     `json:"idempotencyKey,omitempty"`
	Status         string     `json:"status"`
	Attempt        int        `json:"attempt"`
	MaxAttempts    int        `json:"maxAttempts"`
	ScheduledAt    string     `json:"scheduledAt"`
	NextAttemptAt  string     `json:"nextAttemptAt,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	Result         *JobResult `json:"result,omitempty"`
	CreatedAt      string     `json:"createdAt"`
	UpdatedAt      string     `json:"updatedAt"`
	CompletedAt    string     `json:"completedAt,omitempty"`
}

type MessageJobCreate struct {
	ID, InstanceID, Kind, Recipient, MediaPath, Fingerprint, IdempotencyKey string
	Payload                                                                 JobPayload
	ScheduledAt                                                             time.Time
	IdempotencySchedule                                                     string
	MaxAttempts                                                             int
}

func (s *Store) CreateMessageJob(ctx context.Context, input MessageJobCreate) (MessageJob, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MessageJob{}, false, fmt.Errorf("begin message job: %w", err)
	}
	defer tx.Rollback()
	if input.IdempotencyKey != "" {
		existing, getErr := getMessageJobByIdempotency(ctx, tx, input.InstanceID, input.IdempotencyKey)
		if getErr == nil {
			if existing.Fingerprint != input.Fingerprint {
				return MessageJob{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
		if !errors.Is(getErr, ErrJobNotFound) {
			return MessageJob{}, false, getErr
		}
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return MessageJob{}, false, fmt.Errorf("encode message job payload: %w", err)
	}
	now := time.Now().UTC()
	if input.ScheduledAt.IsZero() || input.ScheduledAt.Before(now) {
		input.ScheduledAt = now
	}
	if input.MaxAttempts < 1 {
		input.MaxAttempts = 5
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO message_jobs
(id, instance_id, kind, recipient, payload_json, media_path, fingerprint, idempotency_key, status, attempt, max_attempts,
 scheduled_at, next_attempt_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'queued', 0, ?, ?, ?, ?, ?)`, input.ID, input.InstanceID, input.Kind, input.Recipient,
		string(payload), input.MediaPath, input.Fingerprint, input.IdempotencyKey, input.MaxAttempts,
		input.ScheduledAt.UnixMilli(), input.ScheduledAt.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return MessageJob{}, false, fmt.Errorf("create message job: %w", err)
	}
	job, err := getMessageJob(ctx, tx, input.InstanceID, input.ID)
	if err != nil {
		return MessageJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return MessageJob{}, false, fmt.Errorf("commit message job: %w", err)
	}
	return job, false, nil
}

func (s *Store) GetMessageJob(ctx context.Context, instanceID, id string) (MessageJob, error) {
	return getMessageJob(ctx, s.db, instanceID, id)
}

func getMessageJob(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, instanceID, id string) (MessageJob, error) {
	return scanMessageJob(queryer.QueryRowContext(ctx, messageJobSelect+" WHERE instance_id = ? AND id = ?", instanceID, id))
}

func getMessageJobByIdempotency(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, instanceID, key string) (MessageJob, error) {
	return scanMessageJob(queryer.QueryRowContext(ctx, messageJobSelect+" WHERE instance_id = ? AND idempotency_key = ?", instanceID, key))
}

const messageJobSelect = `SELECT id, instance_id, kind, recipient, payload_json, media_path, fingerprint, idempotency_key,
status, attempt, max_attempts, scheduled_at, next_attempt_at, last_error, result_json, created_at, updated_at, completed_at
FROM message_jobs`

func scanMessageJob(scanner interface{ Scan(...any) error }) (MessageJob, error) {
	var item MessageJob
	var payload, result string
	var scheduledAt, nextAttemptAt, createdAt, updatedAt, completedAt int64
	err := scanner.Scan(&item.ID, &item.InstanceID, &item.Kind, &item.Recipient, &payload, &item.MediaPath,
		&item.Fingerprint, &item.IdempotencyKey, &item.Status, &item.Attempt, &item.MaxAttempts,
		&scheduledAt, &nextAttemptAt, &item.LastError, &result, &createdAt, &updatedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageJob{}, ErrJobNotFound
	}
	if err != nil {
		return MessageJob{}, fmt.Errorf("scan message job: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &item.Payload); err != nil {
		return MessageJob{}, fmt.Errorf("decode message job payload: %w", err)
	}
	if result != "" {
		item.Result = &JobResult{}
		if err := json.Unmarshal([]byte(result), item.Result); err != nil {
			return MessageJob{}, fmt.Errorf("decode message job result: %w", err)
		}
	}
	item.ScheduledAt = formatJobTime(scheduledAt)
	item.NextAttemptAt = formatJobTime(nextAttemptAt)
	item.CreatedAt = formatJobTime(createdAt)
	item.UpdatedAt = formatJobTime(updatedAt)
	item.CompletedAt = formatJobTime(completedAt)
	return item, nil
}

func formatJobTime(value int64) string {
	if value == 0 {
		return ""
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339Nano)
}

func (s *Store) ClaimNextMessageJob(ctx context.Context, now time.Time) (MessageJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MessageJob{}, err
	}
	defer tx.Rollback()
	job, err := scanMessageJob(tx.QueryRowContext(ctx, messageJobSelect+`
WHERE status IN ('queued', 'retrying') AND next_attempt_at <= ?
ORDER BY next_attempt_at ASC, created_at ASC LIMIT 1`, now.UTC().UnixMilli()))
	if err != nil {
		return MessageJob{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE message_jobs SET status = 'processing', attempt = attempt + 1, updated_at = ?
WHERE id = ? AND status IN ('queued', 'retrying')`, now.UTC().UnixMilli(), job.ID)
	if err != nil {
		return MessageJob{}, fmt.Errorf("claim message job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return MessageJob{}, ErrJobStateConflict
	}
	job.Status = "processing"
	job.Attempt++
	job.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	if err := tx.Commit(); err != nil {
		return MessageJob{}, fmt.Errorf("commit claimed message job: %w", err)
	}
	return job, nil
}

func (s *Store) CompleteMessageJob(ctx context.Context, id string, result JobResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	updated, err := s.db.ExecContext(ctx, `UPDATE message_jobs SET status = 'sent', result_json = ?, last_error = '', updated_at = ?, completed_at = ?
WHERE id = ? AND status = 'processing'`, string(raw), now, now, id)
	return jobUpdateResult(updated, err)
}

func (s *Store) FailMessageJob(ctx context.Context, id, message string, retryAt time.Time, terminal bool) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	status, completedAt := "retrying", int64(0)
	if terminal {
		status, completedAt = "failed", time.Now().UTC().UnixMilli()
	}
	now := time.Now().UTC().UnixMilli()
	updated, err := s.db.ExecContext(ctx, `UPDATE message_jobs SET status = ?, last_error = ?, next_attempt_at = ?, updated_at = ?, completed_at = ?
WHERE id = ? AND status = 'processing'`, status, message, retryAt.UTC().UnixMilli(), now, completedAt, id)
	return jobUpdateResult(updated, err)
}

func jobUpdateResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrJobStateConflict
	}
	return nil
}

func (s *Store) RecoverMessageJobs(ctx context.Context) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.ExecContext(ctx, `UPDATE message_jobs SET status = 'retrying', next_attempt_at = ?, updated_at = ?,
last_error = CASE WHEN last_error = '' THEN 'service restarted during delivery' ELSE last_error END
WHERE status = 'processing'`, now, now)
	return err
}

func (s *Store) ListMessageJobs(ctx context.Context, instanceID, status string, page, pageSize int) ([]MessageJob, int, error) {
	page, pageSize = normalizePage(page, pageSize)
	condition := ""
	args := []any{instanceID}
	switch status {
	case "", "all":
	case "queued", "processing", "retrying", "sent", "failed", "canceled":
		condition = " AND status = ?"
		args = append(args, status)
	default:
		return nil, 0, ErrMessageJobFilter
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM message_jobs WHERE instance_id = ?"+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, messageJobSelect+" WHERE instance_id = ?"+condition+" ORDER BY created_at DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]MessageJob, 0, pageSize)
	for rows.Next() {
		item, err := scanMessageJob(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) CancelMessageJob(ctx context.Context, instanceID, id string) (MessageJob, error) {
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `UPDATE message_jobs SET status = 'canceled', updated_at = ?, completed_at = ?
WHERE instance_id = ? AND id = ? AND status IN ('queued', 'retrying')`, now, now, instanceID, id)
	if err != nil {
		return MessageJob{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return MessageJob{}, err
	}
	if count != 1 {
		if _, getErr := s.GetMessageJob(ctx, instanceID, id); errors.Is(getErr, ErrJobNotFound) {
			return MessageJob{}, ErrJobNotFound
		}
		return MessageJob{}, ErrJobStateConflict
	}
	return s.GetMessageJob(ctx, instanceID, id)
}

func (s *Store) RetryMessageJob(ctx context.Context, instanceID, id string) (MessageJob, error) {
	now := time.Now().UTC().UnixMilli()
	result, err := s.db.ExecContext(ctx, `UPDATE message_jobs SET status = 'queued', attempt = 0, next_attempt_at = ?, scheduled_at = ?,
last_error = '', result_json = '', updated_at = ?, completed_at = 0
WHERE instance_id = ? AND id = ? AND status = 'failed'`, now, now, now, instanceID, id)
	if err != nil {
		return MessageJob{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return MessageJob{}, err
	}
	if count != 1 {
		if _, getErr := s.GetMessageJob(ctx, instanceID, id); errors.Is(getErr, ErrJobNotFound) {
			return MessageJob{}, ErrJobNotFound
		}
		return MessageJob{}, ErrJobStateConflict
	}
	return s.GetMessageJob(ctx, instanceID, id)
}

func (s *Store) MessageJobMediaPaths(ctx context.Context, instanceID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT media_path FROM message_jobs WHERE instance_id = ? AND media_path != ''`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func (s *Store) PruneMessageJobs(ctx context.Context, retention time.Duration) ([]string, error) {
	cutoff := time.Now().UTC().Add(-retention).UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT media_path FROM message_jobs
WHERE status IN ('sent', 'failed', 'canceled') AND completed_at > 0 AND completed_at < ? AND media_path != ''`, cutoff)
	if err != nil {
		return nil, err
	}
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_jobs
WHERE status IN ('sent', 'failed', 'canceled') AND completed_at > 0 AND completed_at < ?`, cutoff); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return paths, nil
}

func ValidMessageJobStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "", "all", "queued", "processing", "retrying", "sent", "failed", "canceled":
		return true
	default:
		return false
	}
}
