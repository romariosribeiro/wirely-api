package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type StatusTextPayload struct {
	Text       string `json:"text"`
	Background uint32 `json:"background,omitempty"`
	TextColor  uint32 `json:"textColor,omitempty"`
	Font       int32  `json:"font,omitempty"`
}

func (m *Manager) SendStatusText(ctx context.Context, id string, payload StatusTextPayload) (SentMessage, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return SentMessage{}, err
	}
	payload.Text = strings.TrimSpace(payload.Text)
	if payload.Text == "" || utf8.RuneCountInString(payload.Text) > 700 {
		return SentMessage{}, errors.New("text is required and must have at most 700 characters")
	}
	if payload.Font < 0 || payload.Font > 10 {
		return SentMessage{}, errors.New("font must be between 0 and 10")
	}
	if payload.Background == 0 {
		payload.Background = 0xff1f7aec
	}
	if payload.TextColor == 0 {
		payload.TextColor = 0xffffffff
	}
	font := waE2E.ExtendedTextMessage_FontType(payload.Font)
	message := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(payload.Text), BackgroundArgb: proto.Uint32(payload.Background), TextArgb: proto.Uint32(payload.TextColor), Font: &font}}
	return m.sendStatus(ctx, id, current, "status-text", message, map[string]any{"text": payload.Text})
}

func (m *Manager) SendStatusMedia(ctx context.Context, id string, payload MediaPayload) (SentMessage, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return SentMessage{}, err
	}
	if payload.Kind != MediaImage && payload.Kind != MediaVideo {
		return SentMessage{}, errors.New("status media type must be image or video")
	}
	payload.ViewOnce = false
	payload.Options = MessageOptions{}
	if err := ValidateMedia(&payload); err != nil {
		return SentMessage{}, err
	}
	uploadType := whatsmeow.MediaImage
	if payload.Kind == MediaVideo {
		uploadType = whatsmeow.MediaVideo
	}
	upload, err := current.client.Upload(ctx, payload.Data, uploadType)
	if err != nil {
		return SentMessage{}, fmt.Errorf("upload WhatsApp status media: %w", err)
	}
	return m.sendStatus(ctx, id, current, "status-"+string(payload.Kind), uploadedMessage(payload, upload), map[string]any{"text": payload.Caption, "mimetype": payload.MIMEType, "fileName": payload.FileName, "fileSize": len(payload.Data)})
}

func (m *Manager) sendStatus(ctx context.Context, id string, current *session, kind string, message *waE2E.Message, fields map[string]any) (SentMessage, error) {
	response, err := current.client.SendMessage(ctx, types.StatusBroadcastJID, message)
	if err != nil {
		return SentMessage{}, fmt.Errorf("send WhatsApp status: %w", err)
	}
	timestamp := response.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	eventFields := map[string]any{"id": string(response.ID), "chat": types.StatusBroadcastJID.String(), "fromMe": true, "isGroup": false, "type": kind}
	for key, value := range fields {
		eventFields[key] = value
	}
	m.emit(newEvent("message.sent", id, timestamp, eventFields))
	return SentMessage{ID: string(response.ID), Recipient: types.StatusBroadcastJID.String(), Timestamp: timestamp.Format(time.RFC3339), Type: kind}, nil
}
