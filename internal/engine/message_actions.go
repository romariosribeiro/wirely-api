package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MessageActionResult struct {
	MessageID string `json:"messageId"`
	Chat      string `json:"chat"`
	Status    string `json:"status"`
}

type ChatActionResult struct {
	Chat   string `json:"chat"`
	Action string `json:"action"`
	Status string `json:"status"`
}

func (m *Manager) DeleteMessage(ctx context.Context, id, chat, messageID, participant string) (MessageActionResult, error) {
	current, jid, err := m.messageActionSession(id, chat, messageID)
	if err != nil {
		return MessageActionResult{}, err
	}
	sender, err := actionParticipant(jid, participant)
	if err != nil {
		return MessageActionResult{}, err
	}
	if _, err := current.client.SendMessage(ctx, jid, current.client.BuildRevoke(jid, sender, types.MessageID(messageID))); err != nil {
		return MessageActionResult{}, fmt.Errorf("delete WhatsApp message: %w", err)
	}
	return MessageActionResult{MessageID: messageID, Chat: jid.String(), Status: "deleted"}, nil
}

func (m *Manager) EditMessage(ctx context.Context, id, chat, messageID, message string) (MessageActionResult, error) {
	current, jid, err := m.messageActionSession(id, chat, messageID)
	if err != nil {
		return MessageActionResult{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return MessageActionResult{}, errors.New("message is required")
	}
	if len([]rune(message)) > 4096 {
		return MessageActionResult{}, errors.New("message must have at most 4096 characters")
	}
	content := &waE2E.Message{Conversation: proto.String(message)}
	if _, err := current.client.SendMessage(ctx, jid, current.client.BuildEdit(jid, types.MessageID(messageID), content)); err != nil {
		return MessageActionResult{}, fmt.Errorf("edit WhatsApp message: %w", err)
	}
	return MessageActionResult{MessageID: messageID, Chat: jid.String(), Status: "edited"}, nil
}

func (m *Manager) MarkMessagesRead(ctx context.Context, id, chat string, messageIDs []string, participant string) (MessageActionResult, error) {
	if len(messageIDs) == 0 || len(messageIDs) > 100 {
		return MessageActionResult{}, errors.New("messageIds must contain between 1 and 100 items")
	}
	ids := make([]types.MessageID, len(messageIDs))
	for index, messageID := range messageIDs {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" || len(messageID) > 200 {
			return MessageActionResult{}, errors.New("messageIds contains an invalid message ID")
		}
		ids[index] = types.MessageID(messageID)
	}
	current, err := m.connectedSession(id)
	if err != nil {
		return MessageActionResult{}, err
	}
	jid, _, err := parseMessageRecipient(chat)
	if err != nil {
		return MessageActionResult{}, err
	}
	sender, err := readParticipant(jid, participant)
	if err != nil {
		return MessageActionResult{}, err
	}
	if err := current.client.MarkRead(ctx, ids, time.Now(), jid, sender); err != nil {
		return MessageActionResult{}, fmt.Errorf("mark WhatsApp messages as read: %w", err)
	}
	return MessageActionResult{MessageID: messageIDs[0], Chat: jid.String(), Status: "read"}, nil
}

func (m *Manager) ArchiveChat(ctx context.Context, id, chat string, archived bool) (ChatActionResult, error) {
	current, jid, err := m.chatActionSession(id, chat)
	if err != nil {
		return ChatActionResult{}, err
	}
	if err := current.client.SendAppState(ctx, appstate.BuildArchive(jid, archived, time.Time{}, nil)); err != nil {
		return ChatActionResult{}, fmt.Errorf("archive WhatsApp chat: %w", err)
	}
	status := "unarchived"
	if archived {
		status = "archived"
	}
	return ChatActionResult{Chat: jid.String(), Action: "archive", Status: status}, nil
}

func (m *Manager) MuteChat(ctx context.Context, id, chat string, duration time.Duration) (ChatActionResult, error) {
	if duration < 0 {
		return ChatActionResult{}, errors.New("duration must be zero or greater")
	}
	current, jid, err := m.chatActionSession(id, chat)
	if err != nil {
		return ChatActionResult{}, err
	}
	if err := current.client.SendAppState(ctx, appstate.BuildMute(jid, true, duration)); err != nil {
		return ChatActionResult{}, fmt.Errorf("mute WhatsApp chat: %w", err)
	}
	return ChatActionResult{Chat: jid.String(), Action: "mute", Status: "muted"}, nil
}

func (m *Manager) PinChat(ctx context.Context, id, chat string, pinned bool) (ChatActionResult, error) {
	current, jid, err := m.chatActionSession(id, chat)
	if err != nil {
		return ChatActionResult{}, err
	}
	if err := current.client.SendAppState(ctx, appstate.BuildPin(jid, pinned)); err != nil {
		return ChatActionResult{}, fmt.Errorf("update WhatsApp chat pin: %w", err)
	}
	status := "unpinned"
	action := "unpin"
	if pinned {
		status, action = "pinned", "pin"
	}
	return ChatActionResult{Chat: jid.String(), Action: action, Status: status}, nil
}

func (m *Manager) messageActionSession(id, chat, messageID string) (*session, types.JID, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" || len(messageID) > 200 {
		return nil, types.EmptyJID, errors.New("messageId is required and must have at most 200 characters")
	}
	return m.chatActionSession(id, chat)
}

func (m *Manager) chatActionSession(id, chat string) (*session, types.JID, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, types.EmptyJID, err
	}
	jid, _, err := parseMessageRecipient(chat)
	if err != nil {
		return nil, types.EmptyJID, err
	}
	return current, jid, nil
}

func actionParticipant(chat types.JID, value string) (types.JID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return types.EmptyJID, nil
	}
	if chat.Server != types.GroupServer {
		return types.EmptyJID, errors.New("participant is only supported for group messages")
	}
	participant, _, err := parseMessageRecipient(value)
	if err != nil {
		return types.EmptyJID, fmt.Errorf("invalid participant: %w", err)
	}
	return participant, nil
}

func readParticipant(chat types.JID, value string) (types.JID, error) {
	value = strings.TrimSpace(value)
	if chat.Server != types.GroupServer {
		if value != "" {
			return types.EmptyJID, errors.New("participant is only supported for group messages")
		}
		return chat, nil
	}
	if value == "" {
		return types.EmptyJID, errors.New("participant is required for group messages")
	}
	participant, _, err := parseMessageRecipient(value)
	if err != nil {
		return types.EmptyJID, fmt.Errorf("invalid participant: %w", err)
	}
	return participant, nil
}
