package storage

import (
	"context"
	"fmt"
	"math"
	"time"
)

type InstanceOverview struct {
	Total     int `json:"total"`
	Connected int `json:"connected"`
}

type MessageMetrics struct {
	Sent     int `json:"sent"`
	Received int `json:"received"`
	Total    int `json:"total"`
}

type QueueMetrics struct {
	Queued     int `json:"queued"`
	Processing int `json:"processing"`
	Retrying   int `json:"retrying"`
	Pending    int `json:"pending"`
	Sent       int `json:"sent"`
	Failed     int `json:"failed"`
	Canceled   int `json:"canceled"`
}

type WebhookMetrics struct {
	Delivered     int     `json:"delivered"`
	Failed        int     `json:"failed"`
	Total         int     `json:"total"`
	SuccessRate   float64 `json:"successRate"`
	AverageTimeMS float64 `json:"averageTimeMs"`
}

type InstanceMetrics struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   string         `json:"status"`
	Messages MessageMetrics `json:"messages"`
	Queue    QueueMetrics   `json:"queue"`
	Webhooks WebhookMetrics `json:"webhooks"`
}

type MetricsSnapshot struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Generated  string            `json:"generatedAt"`
	Instances  InstanceOverview  `json:"instances"`
	Messages   MessageMetrics    `json:"messages"`
	Queue      QueueMetrics      `json:"queue"`
	Webhooks   WebhookMetrics    `json:"webhooks"`
	ByInstance []InstanceMetrics `json:"byInstance"`
}

func (s *Store) Metrics(ctx context.Context, from, to time.Time) (MetricsSnapshot, error) {
	from, to = from.UTC(), to.UTC()
	if from.IsZero() || !from.Before(to) {
		return MetricsSnapshot{}, fmt.Errorf("invalid metrics time range")
	}
	snapshot := MetricsSnapshot{
		From: from.Format(time.RFC3339), To: to.Format(time.RFC3339), Generated: time.Now().UTC().Format(time.RFC3339),
		ByInstance: make([]InstanceMetrics, 0),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("begin metrics snapshot: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, "SELECT id, name, status FROM instances ORDER BY name COLLATE NOCASE")
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("list metric instances: %w", err)
	}
	index := make(map[string]int)
	for rows.Next() {
		var item InstanceMetrics
		if err := rows.Scan(&item.ID, &item.Name, &item.Status); err != nil {
			rows.Close()
			return MetricsSnapshot{}, err
		}
		index[item.ID] = len(snapshot.ByInstance)
		snapshot.ByInstance = append(snapshot.ByInstance, item)
		snapshot.Instances.Total++
		if item.Status == "connected" {
			snapshot.Instances.Connected++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MetricsSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return MetricsSnapshot{}, err
	}

	rows, err = tx.QueryContext(ctx, `SELECT instance_id,
COALESCE(SUM(CASE WHEN event = 'message.sent' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN event = 'message.received' THEN 1 ELSE 0 END), 0)
FROM activity_events WHERE occurred_at >= ? AND occurred_at < ? GROUP BY instance_id`, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read message metrics: %w", err)
	}
	for rows.Next() {
		var id string
		var sent, received int
		if err := rows.Scan(&id, &sent, &received); err != nil {
			rows.Close()
			return MetricsSnapshot{}, err
		}
		if position, ok := index[id]; ok {
			snapshot.ByInstance[position].Messages = MessageMetrics{Sent: sent, Received: received, Total: sent + received}
		}
		snapshot.Messages.Sent += sent
		snapshot.Messages.Received += received
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MetricsSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return MetricsSnapshot{}, err
	}
	snapshot.Messages.Total = snapshot.Messages.Sent + snapshot.Messages.Received

	rows, err = tx.QueryContext(ctx, `SELECT instance_id,
COALESCE(SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'retrying' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'sent' AND completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'failed' AND completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'canceled' AND completed_at >= ? AND completed_at < ? THEN 1 ELSE 0 END), 0)
FROM message_jobs GROUP BY instance_id`, from.UnixMilli(), to.UnixMilli(), from.UnixMilli(), to.UnixMilli(), from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read queue metrics: %w", err)
	}
	for rows.Next() {
		var id string
		var item QueueMetrics
		if err := rows.Scan(&id, &item.Queued, &item.Processing, &item.Retrying, &item.Sent, &item.Failed, &item.Canceled); err != nil {
			rows.Close()
			return MetricsSnapshot{}, err
		}
		item.Pending = item.Queued + item.Processing + item.Retrying
		if position, ok := index[id]; ok {
			snapshot.ByInstance[position].Queue = item
		}
		addQueueMetrics(&snapshot.Queue, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MetricsSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return MetricsSnapshot{}, err
	}

	rows, err = tx.QueryContext(ctx, `SELECT instance_id,
COALESCE(SUM(CASE WHEN status = 'delivered' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
COALESCE(AVG(duration_ms), 0)
FROM webhook_deliveries WHERE created_at >= ? AND created_at < ? GROUP BY instance_id`, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("read webhook metrics: %w", err)
	}
	var weightedDuration float64
	for rows.Next() {
		var id string
		var item WebhookMetrics
		if err := rows.Scan(&id, &item.Delivered, &item.Failed, &item.AverageTimeMS); err != nil {
			rows.Close()
			return MetricsSnapshot{}, err
		}
		finishWebhookMetrics(&item)
		if position, ok := index[id]; ok {
			snapshot.ByInstance[position].Webhooks = item
		}
		snapshot.Webhooks.Delivered += item.Delivered
		snapshot.Webhooks.Failed += item.Failed
		weightedDuration += item.AverageTimeMS * float64(item.Total)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return MetricsSnapshot{}, err
	}
	if err := rows.Close(); err != nil {
		return MetricsSnapshot{}, err
	}
	finishWebhookMetrics(&snapshot.Webhooks)
	if snapshot.Webhooks.Total > 0 {
		snapshot.Webhooks.AverageTimeMS = roundMetric(weightedDuration / float64(snapshot.Webhooks.Total))
	}
	if err := tx.Commit(); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("commit metrics snapshot: %w", err)
	}
	return snapshot, nil
}

func addQueueMetrics(total *QueueMetrics, item QueueMetrics) {
	total.Queued += item.Queued
	total.Processing += item.Processing
	total.Retrying += item.Retrying
	total.Pending += item.Pending
	total.Sent += item.Sent
	total.Failed += item.Failed
	total.Canceled += item.Canceled
}

func finishWebhookMetrics(item *WebhookMetrics) {
	item.Total = item.Delivered + item.Failed
	if item.Total > 0 {
		item.SuccessRate = roundMetric(float64(item.Delivered) * 100 / float64(item.Total))
		item.AverageTimeMS = roundMetric(item.AverageTimeMS)
	}
}

func roundMetric(value float64) float64 {
	return math.Round(value*10) / 10
}
