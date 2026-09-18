package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func (m *Manager) ForwardMessage(ctx context.Context, id, sourceChat, messageID, recipient string) (SentMessage, error) {
	message, err := m.store.FindChatMessage(ctx, id, sourceChat, messageID)
	if err != nil {
		if errors.Is(err, storage.ErrChatMessageNotFound) {
			return SentMessage{}, errors.New("source message was not found in the local history")
		}
		return SentMessage{}, fmt.Errorf("load source message: %w", err)
	}
	if media, ok := message.Data["media"].(map[string]any); ok {
		return m.forwardCachedMedia(ctx, id, message, media, recipient)
	}
	if strings.TrimSpace(message.Text) == "" {
		return SentMessage{}, errors.New("this stored message type cannot be forwarded yet")
	}
	return m.SendTextAdvanced(ctx, id, recipient, message.Text, MessageOptions{Forwarded: true})
}

func (m *Manager) forwardCachedMedia(ctx context.Context, id string, message storage.ChatMessage, metadata map[string]any, recipient string) (SentMessage, error) {
	media, err := m.OpenReceivedMedia(ctx, id, message.MessageID)
	if err != nil {
		if errors.Is(err, ErrReceivedMediaNotFound) {
			return SentMessage{}, errors.New("source media is no longer available in the local cache")
		}
		return SentMessage{}, err
	}
	defer media.File.Close()
	if media.Size > MaxMediaBytes {
		return SentMessage{}, fmt.Errorf("source media exceeds the %d MB forwarding limit", MaxMediaBytes>>20)
	}
	data, err := io.ReadAll(io.LimitReader(media.File, MaxMediaBytes+1))
	if err != nil {
		return SentMessage{}, fmt.Errorf("read source media: %w", err)
	}
	payload := MediaPayload{Kind: MediaKind(media.Kind), Data: data, MIMEType: media.MIMEType, FileName: media.FileName, Options: MessageOptions{Forwarded: true}}
	if caption, ok := metadata["caption"].(string); ok {
		payload.Caption = caption
	}
	if voice, ok := metadata["voice"].(bool); ok {
		payload.Voice = voice
	}
	return m.SendMedia(ctx, id, recipient, payload)
}
