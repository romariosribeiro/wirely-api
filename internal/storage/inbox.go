package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var ErrChatMessageNotFound = fmt.Errorf("chat message not found")

type ChatMessage struct {
	EventID   string         `json:"eventId"`
	MessageID string         `json:"messageId,omitempty"`
	Event     string         `json:"event"`
	Chat      string         `json:"chat"`
	Sender    string         `json:"sender,omitempty"`
	FromMe    bool           `json:"fromMe"`
	IsGroup   bool           `json:"isGroup"`
	PushName  string         `json:"pushName,omitempty"`
	Type      string         `json:"type,omitempty"`
	Text      string         `json:"text,omitempty"`
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

type ChatSummary struct {
	Chat        string      `json:"chat"`
	Name        string      `json:"name"`
	IsGroup     bool        `json:"isGroup"`
	UnreadCount int         `json:"unreadCount"`
	LastMessage ChatMessage `json:"lastMessage"`
}

func saveChatMessage(ctx context.Context, tx *sql.Tx, eventID, instanceID, event string, occurredAt time.Time, data map[string]any, raw string) error {
	if !strings.HasPrefix(event, "message.") || event == "message.receipt" {
		return nil
	}
	chat := dataString(data, "chat")
	if chat == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO chat_messages
(event_id, instance_id, message_id, event, chat, sender, from_me, is_group, push_name, message_type, text, timestamp, data_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, eventID, instanceID, dataString(data, "id"), event, chat,
		dataString(data, "from"), dataBool(data, "fromMe"), dataBool(data, "isGroup"), dataString(data, "pushName"),
		dataString(data, "type"), dataString(data, "text"), occurredAt.UnixMilli(), raw)
	if err != nil {
		return fmt.Errorf("save chat message: %w", err)
	}
	return nil
}

func dataString(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func dataBool(data map[string]any, key string) bool {
	value, _ := data[key].(bool)
	return value
}

func (s *Store) ListChats(ctx context.Context, instanceID, search string, page, pageSize int) ([]ChatSummary, int, error) {
	return s.ListChatsIncluding(ctx, instanceID, search, nil, page, pageSize)
}

func (s *Store) ListChatsIncluding(ctx context.Context, instanceID, search string, includedChats []string, page, pageSize int) ([]ChatSummary, int, error) {
	page, pageSize = normalizePage(page, pageSize)
	needle := "%" + strings.ToLower(strings.TrimSpace(search)) + "%"
	extraCondition := ""
	for range includedChats {
		extraCondition += ", ?"
	}
	if extraCondition != "" {
		extraCondition = " OR chat IN (" + strings.TrimPrefix(extraCondition, ", ") + ")"
	}
	condition := "(LOWER(chat) LIKE ? OR LOWER(name) LIKE ?" + extraCondition + ")"
	countQuery := `WITH rollup AS (
    SELECT chat, MAX(push_name) AS name
    FROM chat_messages WHERE instance_id = ? GROUP BY chat
)
SELECT COUNT(*) FROM rollup WHERE ` + condition
	countArgs := []any{instanceID, needle, needle}
	for _, chat := range includedChats {
		countArgs = append(countArgs, chat)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chats: %w", err)
	}
	listCondition := strings.ReplaceAll(condition, "LOWER(chat)", "LOWER(rollup.chat)")
	listCondition = strings.ReplaceAll(listCondition, "LOWER(name)", "LOWER(rollup.name)")
	listCondition = strings.Replace(listCondition, "chat IN", "rollup.chat IN", 1)
	query := `WITH latest AS (
    SELECT event_id, message_id, event, chat, sender, from_me, is_group, push_name, message_type, text, timestamp, data_json,
           ROW_NUMBER() OVER (PARTITION BY chat ORDER BY timestamp DESC, event_id DESC) AS row_number
    FROM chat_messages WHERE instance_id = ?
), rollup AS (
    SELECT messages.chat, MAX(messages.push_name) AS name, MAX(messages.is_group) AS is_group,
           SUM(CASE WHEN messages.event = 'message.received' AND messages.timestamp > COALESCE(reads.read_at, 0) THEN 1 ELSE 0 END) AS unread_count
    FROM chat_messages AS messages
    LEFT JOIN chat_reads AS reads ON reads.instance_id = messages.instance_id AND reads.chat = messages.chat
    WHERE messages.instance_id = ? GROUP BY messages.chat
)
SELECT rollup.chat, rollup.name, rollup.is_group, rollup.unread_count,
       latest.event_id, latest.message_id, latest.event, latest.chat, latest.sender, latest.from_me,
       latest.is_group, latest.push_name, latest.message_type, latest.text, latest.timestamp, latest.data_json
FROM rollup JOIN latest ON latest.chat = rollup.chat AND latest.row_number = 1
WHERE ` + listCondition + `
ORDER BY latest.timestamp DESC, latest.event_id DESC LIMIT ? OFFSET ?`
	queryArgs := []any{instanceID, instanceID, needle, needle}
	for _, chat := range includedChats {
		queryArgs = append(queryArgs, chat)
	}
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()
	items := make([]ChatSummary, 0, pageSize)
	for rows.Next() {
		var item ChatSummary
		var group int
		var messageGroup int
		var fromMe int
		var timestamp int64
		var raw string
		if err := rows.Scan(&item.Chat, &item.Name, &group, &item.UnreadCount,
			&item.LastMessage.EventID, &item.LastMessage.MessageID, &item.LastMessage.Event, &item.LastMessage.Chat,
			&item.LastMessage.Sender, &fromMe, &messageGroup, &item.LastMessage.PushName, &item.LastMessage.Type,
			&item.LastMessage.Text, &timestamp, &raw); err != nil {
			return nil, 0, fmt.Errorf("scan chat: %w", err)
		}
		item.IsGroup = group != 0
		if item.IsGroup {
			item.Name = ""
		}
		item.LastMessage.FromMe = fromMe != 0
		item.LastMessage.IsGroup = messageGroup != 0
		item.LastMessage.Timestamp = time.UnixMilli(timestamp).UTC().Format(time.RFC3339Nano)
		if err := json.Unmarshal([]byte(raw), &item.LastMessage.Data); err != nil {
			return nil, 0, fmt.Errorf("decode chat message: %w", err)
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) ListChatMessages(ctx context.Context, instanceID, chat string, page, pageSize int) ([]ChatMessage, int, error) {
	page, pageSize = normalizePage(page, pageSize)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chat_messages WHERE instance_id = ? AND chat = ?`, instanceID, chat).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count chat messages: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, message_id, event, chat, sender, from_me, is_group, push_name, message_type, text, timestamp, data_json
FROM chat_messages WHERE instance_id = ? AND chat = ?
ORDER BY timestamp DESC, event_id DESC LIMIT ? OFFSET ?`, instanceID, chat, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()
	items := make([]ChatMessage, 0, pageSize)
	for rows.Next() {
		item, err := scanChatMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) FindChatMessage(ctx context.Context, instanceID, chat, messageID string) (ChatMessage, error) {
	row := s.db.QueryRowContext(ctx, `SELECT event_id, message_id, event, chat, sender, from_me, is_group, push_name, message_type, text, timestamp, data_json
FROM chat_messages WHERE instance_id = ? AND chat = ? AND message_id = ?
ORDER BY timestamp DESC LIMIT 1`, instanceID, strings.TrimSpace(chat), strings.TrimSpace(messageID))
	item, err := scanChatMessage(row)
	if err == sql.ErrNoRows {
		return ChatMessage{}, ErrChatMessageNotFound
	}
	return item, err
}

func scanChatMessage(scanner interface{ Scan(...any) error }) (ChatMessage, error) {
	var item ChatMessage
	var fromMe, isGroup int
	var timestamp int64
	var raw string
	if err := scanner.Scan(&item.EventID, &item.MessageID, &item.Event, &item.Chat, &item.Sender, &fromMe, &isGroup,
		&item.PushName, &item.Type, &item.Text, &timestamp, &raw); err != nil {
		return ChatMessage{}, fmt.Errorf("scan chat message: %w", err)
	}
	item.FromMe = fromMe != 0
	item.IsGroup = isGroup != 0
	item.Timestamp = time.UnixMilli(timestamp).UTC().Format(time.RFC3339Nano)
	if err := json.Unmarshal([]byte(raw), &item.Data); err != nil {
		return ChatMessage{}, fmt.Errorf("decode chat message: %w", err)
	}
	return item, nil
}

func (s *Store) MarkChatRead(ctx context.Context, instanceID, chat string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO chat_reads(instance_id, chat, read_at) VALUES (?, ?, ?)
ON CONFLICT(instance_id, chat) DO UPDATE SET read_at = excluded.read_at`, instanceID, chat, time.Now().UTC().UnixMilli())
	if err != nil {
		return fmt.Errorf("mark chat read: %w", err)
	}
	return nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
