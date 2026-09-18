package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type PresenceState struct {
	Phone     string `json:"phone"`
	JID       string `json:"jid"`
	Presence  string `json:"presence"`
	LastSeen  string `json:"lastSeen,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func (m *Manager) SetPresence(ctx context.Context, id, presence string) (PresenceState, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return PresenceState{}, err
	}
	value := strings.ToLower(strings.TrimSpace(presence))
	var state types.Presence
	switch value {
	case "available", "online":
		state, value = types.PresenceAvailable, "available"
	case "unavailable", "offline":
		state, value = types.PresenceUnavailable, "unavailable"
	default:
		return PresenceState{}, errors.New("presence must be available or unavailable")
	}
	if err := current.client.SendPresence(ctx, state); err != nil {
		return PresenceState{}, fmt.Errorf("set WhatsApp presence: %w", err)
	}
	return PresenceState{Presence: value, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

func (m *Manager) SubscribePresence(ctx context.Context, id, phone string) (PresenceState, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return PresenceState{}, err
	}
	jid, display, err := resolveMessageRecipient(ctx, current.client, phone)
	if err != nil {
		return PresenceState{}, err
	}
	if jid.Server == types.GroupServer {
		return PresenceState{}, errors.New("presence is only supported for individual users")
	}
	if err := current.client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		return PresenceState{}, fmt.Errorf("enable WhatsApp presence: %w", err)
	}
	if err := current.client.SubscribePresence(ctx, jid); err != nil {
		return PresenceState{}, fmt.Errorf("subscribe to WhatsApp presence: %w", err)
	}
	if state, ok := m.cachedPresence(id, jid); ok {
		return state, nil
	}
	return PresenceState{Phone: display, JID: jid.String(), Presence: "unknown"}, nil
}

func (m *Manager) GetPresence(ctx context.Context, id, phone string) (PresenceState, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return PresenceState{}, err
	}
	jid, display, err := resolveMessageRecipient(ctx, current.client, phone)
	if err != nil {
		return PresenceState{}, err
	}
	if state, ok := m.cachedPresence(id, jid); ok {
		return state, nil
	}
	return PresenceState{Phone: display, JID: jid.String(), Presence: "unknown"}, nil
}

func (m *Manager) rememberPresence(id string, event *events.Presence) {
	state := PresenceState{Phone: event.From.User, JID: event.From.ToNonAD().String(), Presence: "available", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if event.Unavailable {
		state.Presence = "unavailable"
	}
	if !event.LastSeen.IsZero() {
		state.LastSeen = event.LastSeen.UTC().Format(time.RFC3339)
	}
	m.presenceMu.Lock()
	m.presences[presenceKey(id, event.From)] = state
	m.presenceMu.Unlock()
}

func (m *Manager) cachedPresence(id string, jid types.JID) (PresenceState, bool) {
	m.presenceMu.RLock()
	state, ok := m.presences[presenceKey(id, jid)]
	m.presenceMu.RUnlock()
	return state, ok
}

func presenceKey(id string, jid types.JID) string { return id + "\x00" + jid.ToNonAD().String() }

func (m *Manager) SendChatPresence(ctx context.Context, id, recipient, presence string) error {
	current, err := m.connectedSession(id)
	if err != nil {
		return err
	}
	jid, _, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return err
	}
	state, media, err := chatPresence(presence)
	if err != nil {
		return err
	}
	if err := current.client.SendChatPresence(ctx, jid, state, media); err != nil {
		return fmt.Errorf("send WhatsApp chat presence: %w", err)
	}
	return nil
}

func chatPresence(value string) (types.ChatPresence, types.ChatPresenceMedia, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "composing":
		return types.ChatPresenceComposing, types.ChatPresenceMediaText, nil
	case "recording":
		return types.ChatPresenceComposing, types.ChatPresenceMediaAudio, nil
	case "paused":
		return types.ChatPresencePaused, types.ChatPresenceMediaText, nil
	default:
		return "", "", errors.New("presence must be composing or recording")
	}
}
