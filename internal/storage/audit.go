package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
)

type AuditEntry struct {
	ID         string         `json:"id"`
	UserID     string         `json:"userId,omitempty"`
	Username   string         `json:"username,omitempty"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	Status     int            `json:"status"`
	SourceIP   string         `json:"sourceIp,omitempty"`
	Details    map[string]any `json:"details"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func (s *Store) RecordAudit(ctx context.Context, entry AuditEntry) error {
	if strings.TrimSpace(entry.Action) == "" {
		return fmt.Errorf("audit action is required")
	}
	if entry.ID == "" {
		random, err := security.RandomToken(12)
		if err != nil {
			return err
		}
		entry.ID = "aud_" + random
	}
	if entry.Details == nil {
		entry.Details = map[string]any{}
	}
	details, err := json.Marshal(entry.Details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO admin_audit
(id, user_id, username, action, target_type, target_id, status, source_ip, details_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, entry.ID, entry.UserID, entry.Username, entry.Action,
		entry.TargetType, entry.TargetID, entry.Status, entry.SourceIP, string(details), entry.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("record admin audit: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(ctx context.Context, page, pageSize int, action string) ([]AuditEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	action = strings.TrimSpace(action)
	where, arguments := "", []any{}
	if action != "" {
		where = " WHERE action = ?"
		arguments = append(arguments, action)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_audit"+where, arguments...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit entries: %w", err)
	}
	arguments = append(arguments, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, username, action, target_type, target_id,
status, source_ip, details_json, created_at FROM admin_audit`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, arguments...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()
	entries := make([]AuditEntry, 0)
	for rows.Next() {
		entry, err := scanAudit(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	return entries, total, rows.Err()
}

func scanAudit(scanner interface{ Scan(...any) error }) (AuditEntry, error) {
	var entry AuditEntry
	var details string
	var createdAt int64
	if err := scanner.Scan(&entry.ID, &entry.UserID, &entry.Username, &entry.Action, &entry.TargetType,
		&entry.TargetID, &entry.Status, &entry.SourceIP, &details, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return AuditEntry{}, err
		}
		return AuditEntry{}, fmt.Errorf("scan audit entry: %w", err)
	}
	entry.CreatedAt = time.Unix(createdAt, 0).UTC()
	entry.Details = map[string]any{}
	if err := json.Unmarshal([]byte(details), &entry.Details); err != nil {
		return AuditEntry{}, fmt.Errorf("decode audit details: %w", err)
	}
	return entry, nil
}
