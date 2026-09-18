package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

var ErrNotConnected = errors.New("instance is not connected")

type SentMessage struct {
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
}

func (m *Manager) SendText(ctx context.Context, id, recipient, message string) (SentMessage, error) {
	return m.SendTextAdvanced(ctx, id, recipient, message, MessageOptions{})
}

func (m *Manager) SendTextAdvanced(ctx context.Context, id, recipient, message string, options MessageOptions) (SentMessage, error) {
	current, err := m.get(id)
	if err != nil {
		return SentMessage{}, err
	}
	if !current.client.IsConnected() || !current.client.IsLoggedIn() {
		return SentMessage{}, ErrNotConnected
	}

	message, err = ValidateText(message)
	if err != nil {
		return SentMessage{}, err
	}

	jid, display, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return SentMessage{}, err
	}
	content, err := textMessage(message, jid, options)
	if err != nil {
		return SentMessage{}, err
	}
	response, err := current.client.SendMessage(ctx, jid, content)
	if err != nil {
		return SentMessage{}, fmt.Errorf("send WhatsApp message: %w", err)
	}
	timestamp := response.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	m.emit(newEvent("message.sent", id, timestamp, map[string]any{
		"id": string(response.ID), "chat": jid.String(), "fromMe": true, "isGroup": jid.Server == types.GroupServer, "type": "text", "text": message,
	}))
	return SentMessage{
		ID: string(response.ID), Recipient: display,
		Timestamp: timestamp.Format(time.RFC3339), Type: "text",
	}, nil
}

func NormalizeRecipient(value string) (string, error) {
	value = strings.TrimSpace(value)
	var normalized strings.Builder
	for index, character := range value {
		switch {
		case character >= '0' && character <= '9':
			normalized.WriteRune(character)
		case character == '+' && index == 0:
		case character == ' ' || character == '-' || character == '(' || character == ')':
		default:
			return "", errors.New("recipient must be a phone number with country code")
		}
	}
	phone := normalized.String()
	if len(phone) < 8 || len(phone) > 15 || phone[0] == '0' {
		return "", errors.New("recipient must contain 8 to 15 digits including country code")
	}
	return phone, nil
}

func normalizePhone(value string) (string, error) {
	return NormalizeRecipient(value)
}

func ValidateText(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("message is required")
	}
	if len([]rune(value)) > 4096 {
		return "", errors.New("message must have at most 4096 characters")
	}
	return value, nil
}
