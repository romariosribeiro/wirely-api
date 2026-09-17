package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

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
