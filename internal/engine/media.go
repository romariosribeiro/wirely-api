package engine

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MediaKind string

const (
	MediaImage    MediaKind = "image"
	MediaVideo    MediaKind = "video"
	MediaAudio    MediaKind = "audio"
	MediaDocument MediaKind = "document"
	MediaSticker  MediaKind = "sticker"
	MaxMediaBytes           = 32 << 20
)

type MediaPayload struct {
	Kind     MediaKind
	Data     []byte
	MIMEType string
	FileName string
	Caption  string
	Voice    bool
	ViewOnce bool
	Options  MessageOptions
}

func (m *Manager) SendMedia(ctx context.Context, id, recipient string, media MediaPayload) (SentMessage, error) {
	current, err := m.get(id)
	if err != nil {
		return SentMessage{}, err
	}
	if !current.client.IsConnected() || !current.client.IsLoggedIn() {
		return SentMessage{}, ErrNotConnected
	}
	if err := ValidateMedia(&media); err != nil {
		return SentMessage{}, err
	}
	jid, display, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return SentMessage{}, err
	}

	uploadType := whatsmeow.MediaDocument
	switch media.Kind {
	case MediaImage, MediaSticker:
		uploadType = whatsmeow.MediaImage
	case MediaVideo:
		uploadType = whatsmeow.MediaVideo
	case MediaAudio:
		uploadType = whatsmeow.MediaAudio
	}
	upload, err := current.client.Upload(ctx, media.Data, uploadType)
	if err != nil {
		return SentMessage{}, fmt.Errorf("upload WhatsApp media: %w", err)
	}
	message := uploadedMessage(media, upload)
	if err := applyMessageOptions(message, jid, media.Options); err != nil {
		return SentMessage{}, err
	}
	if media.ViewOnce {
		message = &waE2E.Message{ViewOnceMessageV2: &waE2E.FutureProofMessage{Message: message}}
	}
	response, err := current.client.SendMessage(ctx, jid, message)
	if err != nil {
		return SentMessage{}, fmt.Errorf("send WhatsApp %s: %w", media.Kind, err)
	}
	timestamp := response.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	m.emit(newEvent("message.sent", id, timestamp, map[string]any{
		"id": string(response.ID), "chat": jid.String(), "fromMe": true, "isGroup": jid.Server == types.GroupServer,
		"type": string(media.Kind), "text": media.Caption, "mimetype": media.MIMEType,
		"fileName": media.FileName, "fileSize": len(media.Data),
	}))
	return SentMessage{ID: string(response.ID), Recipient: display, Timestamp: timestamp.Format(time.RFC3339), Type: string(media.Kind)}, nil
}

func ValidateMedia(media *MediaPayload) error {
	if len(media.Data) == 0 {
		return errors.New("file is required")
	}
	if len(media.Data) > MaxMediaBytes {
		return fmt.Errorf("file must have at most %d MB", MaxMediaBytes>>20)
	}
	media.MIMEType = strings.ToLower(strings.TrimSpace(strings.Split(media.MIMEType, ";")[0]))
	if _, _, err := mime.ParseMediaType(media.MIMEType); err != nil {
		return errors.New("invalid file MIME type")
	}
	switch media.Kind {
	case MediaImage:
		if !strings.HasPrefix(media.MIMEType, "image/") {
			return errors.New("image endpoint requires an image file")
		}
	case MediaVideo:
		if !strings.HasPrefix(media.MIMEType, "video/") {
			return errors.New("video type requires a video file")
		}
	case MediaAudio:
		if media.ViewOnce {
			return errors.New("viewOnce is only supported for image or video")
		}
		if !strings.HasPrefix(media.MIMEType, "audio/") && media.MIMEType != "application/ogg" {
			return errors.New("audio endpoint requires an audio file")
		}
		if media.Voice && media.MIMEType != "audio/ogg" && media.MIMEType != "application/ogg" {
			return errors.New("voice messages require an OGG/Opus audio file")
		}
	case MediaDocument:
		if media.ViewOnce {
			return errors.New("viewOnce is only supported for image or video")
		}
	case MediaSticker:
		if media.ViewOnce {
			return errors.New("viewOnce is only supported for image or video")
		}
		if media.MIMEType != "image/webp" {
			return errors.New("sticker type requires a WebP file")
		}
	default:
		return errors.New("unsupported media type")
	}
	media.Caption = strings.TrimSpace(media.Caption)
	if media.Kind == MediaSticker && media.Caption != "" {
		return errors.New("sticker type does not support captions")
	}
	if len([]rune(media.Caption)) > 1024 {
		return errors.New("caption must have at most 1024 characters")
	}
	media.FileName = safeFileName(media.FileName)
	if media.Kind == MediaDocument && media.FileName == "" {
		return errors.New("document file name is required")
	}
	return nil
}

func safeFileName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > 180 {
		value = string(runes[:180])
	}
	if value == "." {
		return ""
	}
	return value
}

func uploadedMessage(media MediaPayload, upload whatsmeow.UploadResponse) *waE2E.Message {
	switch media.Kind {
	case MediaImage:
		return &waE2E.Message{ImageMessage: &waE2E.ImageMessage{URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), Mimetype: proto.String(media.MIMEType), Caption: proto.String(media.Caption)}}
	case MediaVideo:
		return &waE2E.Message{VideoMessage: &waE2E.VideoMessage{URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), Mimetype: proto.String(media.MIMEType), Caption: proto.String(media.Caption)}}
	case MediaAudio:
		return &waE2E.Message{AudioMessage: &waE2E.AudioMessage{URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), Mimetype: proto.String(media.MIMEType), PTT: proto.Bool(media.Voice)}}
	case MediaSticker:
		return &waE2E.Message{StickerMessage: &waE2E.StickerMessage{URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), Mimetype: proto.String(media.MIMEType)}}
	default:
		return &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{URL: proto.String(upload.URL), DirectPath: proto.String(upload.DirectPath), MediaKey: upload.MediaKey, FileEncSHA256: upload.FileEncSHA256, FileSHA256: upload.FileSHA256, FileLength: proto.Uint64(upload.FileLength), Mimetype: proto.String(media.MIMEType), FileName: proto.String(media.FileName), Title: proto.String(media.FileName), Caption: proto.String(media.Caption)}}
	}
}
