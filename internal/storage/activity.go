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
	ErrActivityFilter       = errors.New("invalid activity filter")
	ErrDeliveryNotFound     = errors.New("webhook delivery not found")
	ErrActivityEventMissing = errors.New("activity event not found")
	ErrMessageStatusMissing = errors.New("message status not found")
)

type ActivityEvent struct {
	ID         string         `json:"id"`
	InstanceID string         `json:"instanceId"`
	Event      string         `json:"event"`
	Timestamp  string         `json:"timestamp"`
	Data       map[string]any `json:"data"`
}

type MessageStatus struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
	Chat      string `json:"chat,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

type WebhookDelivery struct {
	ID         int64  `json:"id"`
	EventID    string `json:"eventId"`
	InstanceID string `json:"instanceId"`
	Event      string `json:"event"`
	URL        string `json:"url"`
	Attempt    int    `json:"attempt"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Manual     bool   `json:"manual"`
	CreatedAt  string `json:"createdAt"`
}

type DeliveryRecord struct {
	EventID, InstanceID, Event, URL, Status, Error string
	Attempt, HTTPStatus                            int
	Duration                                       time.Duration
	Manual                                         bool
}

func (s *Store) SaveActivityEvent(ctx context.Context, id, instanceID, event, timestamp string, data map[string]any) (bool, error) {
	if id == "" || instanceID == "" || event == "" {
		return false, errors.New("activity event identity is required")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return false, fmt.Errorf("encode activity event: %w", err)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		occurredAt = time.Now().UTC()
	}
	dedupeKey := ""
	if strings.HasPrefix(event, "message.") || strings.HasPrefix(event, "status.") {
		if value, ok := data["id"].(string); ok {
			dedupeKey = strings.TrimSpace(value)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin activity event transaction: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO activity_events (id, instance_id, event, occurred_at, data_json, dedupe_key, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, id, instanceID, event, occurredAt.UnixMilli(), string(raw), dedupeKey, time.Now().UTC().UnixMilli())
	if err != nil {
		return false, fmt.Errorf("save activity event: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read activity event result: %w", err)
	}
	if count > 0 {
		if err := saveChatMessage(ctx, tx, id, instanceID, event, occurredAt, data, string(raw)); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit activity event: %w", err)
	}
	return count > 0, nil
}

func (s *Store) GetActivityEvent(ctx context.Context, instanceID, eventID string) (ActivityEvent, error) {
	var item ActivityEvent
	var occurredAt int64
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT id, instance_id, event, occurred_at, data_json
FROM activity_events WHERE instance_id = ? AND id = ?`, instanceID, eventID).Scan(&item.ID, &item.InstanceID, &item.Event, &occurredAt, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivityEvent{}, ErrActivityEventMissing
	}
	if err != nil {
		return ActivityEvent{}, fmt.Errorf("get activity event: %w", err)
	}
	item.Timestamp = time.UnixMilli(occurredAt).UTC().Format(time.RFC3339Nano)
	if err := json.Unmarshal([]byte(raw), &item.Data); err != nil {
		return ActivityEvent{}, fmt.Errorf("decode activity event: %w", err)
	}
	return item, nil
}

func (s *Store) GetMessageStatus(ctx context.Context, instanceID, messageID string) (MessageStatus, error) {
	var occurredAt int64
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT a.occurred_at, a.data_json
FROM activity_events AS a, json_each(a.data_json, '$.ids') AS receipt_id
WHERE a.instance_id = ? AND a.event = 'message.receipt' AND receipt_id.value = ?
ORDER BY a.occurred_at DESC, a.created_at DESC LIMIT 1`, instanceID, messageID).Scan(&occurredAt, &raw)
	if err == nil {
		var data struct {
			Chat string `json:"chat"`
			Type string `json:"type"`
		}
		if decodeErr := json.Unmarshal([]byte(raw), &data); decodeErr != nil {
			return MessageStatus{}, fmt.Errorf("decode message receipt: %w", decodeErr)
		}
		status := strings.TrimSpace(data.Type)
		if status == "" || status == "sender" {
			status = "delivered"
		}
		return MessageStatus{MessageID: messageID, Status: status, Chat: data.Chat, UpdatedAt: time.UnixMilli(occurredAt).UTC().Format(time.RFC3339Nano)}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MessageStatus{}, fmt.Errorf("get message receipt: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
SELECT occurred_at, data_json FROM activity_events
WHERE instance_id = ? AND event IN ('message.sent', 'message.updated', 'message.deleted') AND dedupe_key = ?
ORDER BY occurred_at DESC, created_at DESC LIMIT 1`, instanceID, messageID).Scan(&occurredAt, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return MessageStatus{}, ErrMessageStatusMissing
	}
	if err != nil {
		return MessageStatus{}, fmt.Errorf("get sent message status: %w", err)
	}
	var data struct {
		Chat string `json:"chat"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return MessageStatus{}, fmt.Errorf("decode sent message status: %w", err)
	}
	return MessageStatus{MessageID: messageID, Status: "sent", Chat: data.Chat, UpdatedAt: time.UnixMilli(occurredAt).UTC().Format(time.RFC3339Nano)}, nil
}

func (s *Store) ListActivityEvents(ctx context.Context, instanceID, category string, page, pageSize int) ([]ActivityEvent, int, error) {
	condition, err := activityCondition(category)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	where := "instance_id = ?" + condition
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM activity_events WHERE "+where, instanceID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity events: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, instance_id, event, occurred_at, data_json
FROM activity_events WHERE `+where+` ORDER BY occurred_at DESC, created_at DESC, id DESC LIMIT ? OFFSET ?`, instanceID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list activity events: %w", err)
	}
	defer rows.Close()
	items := make([]ActivityEvent, 0, pageSize)
	for rows.Next() {
		var item ActivityEvent
		var occurredAt int64
		var raw string
		if err := rows.Scan(&item.ID, &item.InstanceID, &item.Event, &occurredAt, &raw); err != nil {
			return nil, 0, err
		}
		item.Timestamp = time.UnixMilli(occurredAt).UTC().Format(time.RFC3339Nano)
		if err := json.Unmarshal([]byte(raw), &item.Data); err != nil {
			return nil, 0, fmt.Errorf("decode activity event: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func activityCondition(category string) (string, error) {
	switch category {
	case "", "all":
		return "", nil
	case "messages":
		return " AND event LIKE 'message.%'", nil
	case "connection":
		return " AND event = 'instance.status'", nil
	case "status":
		return " AND event LIKE 'status.%'", nil
	case "presence":
		return " AND event LIKE 'presence.%'", nil
	case "groups":
		return " AND event LIKE 'group.%'", nil
	default:
		return "", ErrActivityFilter
	}
}

func (s *Store) RecordWebhookDelivery(ctx context.Context, record DeliveryRecord) error {
	if record.Status != "delivered" && record.Status != "failed" {
		return errors.New("invalid webhook delivery status")
	}
	if len(record.Error) > 1000 {
		record.Error = record.Error[:1000]
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO webhook_deliveries
(event_id, instance_id, event, url, attempt, status, http_status, error, duration_ms, manual, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, record.EventID, record.InstanceID, record.Event, record.URL,
		record.Attempt, record.Status, record.HTTPStatus, record.Error, record.Duration.Milliseconds(), record.Manual, time.Now().UTC().UnixMilli())
	if err != nil {
		return fmt.Errorf("record webhook delivery: %w", err)
	}
	return nil
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, instanceID, status string, page, pageSize int) ([]WebhookDelivery, int, error) {
	condition := ""
	switch status {
	case "", "all":
	case "failed", "delivered":
		condition = " AND status = ?"
	default:
		return nil, 0, ErrActivityFilter
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	countArgs := []any{instanceID}
	if condition != "" {
		countArgs = append(countArgs, status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM webhook_deliveries WHERE instance_id = ?"+condition, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(countArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, instance_id, event, url, attempt, status, http_status, error, duration_ms, manual, created_at
FROM webhook_deliveries WHERE instance_id = ?`+condition+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()
	items := make([]WebhookDelivery, 0, pageSize)
	for rows.Next() {
		var item WebhookDelivery
		var manual int
		var createdAt int64
		if err := rows.Scan(&item.ID, &item.EventID, &item.InstanceID, &item.Event, &item.URL, &item.Attempt, &item.Status, &item.HTTPStatus, &item.Error, &item.DurationMS, &manual, &createdAt); err != nil {
			return nil, 0, err
		}
		item.Manual = manual != 0
		item.CreatedAt = time.UnixMilli(createdAt).UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) GetWebhookDelivery(ctx context.Context, instanceID string, id int64) (WebhookDelivery, error) {
	var item WebhookDelivery
	var manual int
	var createdAt int64
	err := s.db.QueryRowContext(ctx, `SELECT id, event_id, instance_id, event, url, attempt, status, http_status, error, duration_ms, manual, created_at
FROM webhook_deliveries WHERE instance_id = ? AND id = ?`, instanceID, id).Scan(&item.ID, &item.EventID, &item.InstanceID, &item.Event, &item.URL, &item.Attempt, &item.Status, &item.HTTPStatus, &item.Error, &item.DurationMS, &manual, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return WebhookDelivery{}, ErrDeliveryNotFound
	}
	if err != nil {
		return WebhookDelivery{}, fmt.Errorf("get webhook delivery: %w", err)
	}
	item.Manual = manual != 0
	item.CreatedAt = time.UnixMilli(createdAt).UTC().Format(time.RFC3339Nano)
	return item, nil
}

func (s *Store) PruneActivity(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().UTC().Add(-retention).UnixMilli()
	if _, err := s.db.ExecContext(ctx, "DELETE FROM webhook_jobs WHERE status IN ('delivered', 'dead') AND updated_at < ?", cutoff); err != nil {
		return fmt.Errorf("prune webhook jobs: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM webhook_deliveries WHERE created_at < ?", cutoff); err != nil {
		return fmt.Errorf("prune webhook deliveries: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM activity_events WHERE created_at < ?", cutoff); err != nil {
		return fmt.Errorf("prune activity events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM chat_reads
WHERE NOT EXISTS (SELECT 1 FROM chat_messages WHERE chat_messages.instance_id = chat_reads.instance_id AND chat_messages.chat = chat_reads.chat)`); err != nil {
		return fmt.Errorf("prune chat read markers: %w", err)
	}
	return nil
}
