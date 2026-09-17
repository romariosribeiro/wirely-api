package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

const MaxReceivedMediaBytes = 100 << 20

var ErrReceivedMediaNotFound = errors.New("received media not found")

type ReceivedMedia struct {
	File     *os.File
	Kind     string
	MIMEType string
	FileName string
	Size     int64
	SavedAt  time.Time
}

type receivedMediaMetadata struct {
	MessageID string `json:"messageId"`
	Kind      string `json:"type"`
	MIMEType  string `json:"mimetype"`
	FileName  string `json:"fileName"`
	Size      int64  `json:"size"`
	SavedAt   string `json:"savedAt"`
}

type incomingMedia struct {
	download whatsmeow.DownloadableMessage
	kind     string
	mimeType string
	fileName string
	declared uint64
	caption  string
	duration uint32
	voice    bool
	animated bool
}

func (m *Manager) prepareIncomingMessage(current *session, message *events.Message) Event {
	event := incomingMessageEvent(current.id, message)
	addStructuredMessageMetadata(event.Data, message.Message)
	media, ok := incomingMediaFromMessage(message.Message)
	if !ok {
		return event
	}
	messageID := string(message.Info.ID)
	media.fileName = receivedMediaFileName(messageID, media.fileName, media.mimeType)
	metadata := map[string]any{
		"available": false, "type": media.kind, "mimetype": media.mimeType,
		"fileName": media.fileName, "size": media.declared,
		"downloadUrl": "/api/messages/" + url.PathEscape(messageID) + "/media",
	}
	if media.caption != "" {
		metadata["caption"] = media.caption
	}
	if media.duration > 0 {
		metadata["duration"] = media.duration
	}
	if media.voice {
		metadata["voice"] = true
	}
	if media.animated {
		metadata["animated"] = true
	}
	event.Data["media"] = metadata
	event.Data["type"] = media.kind
	event.Data["mimetype"] = media.mimeType
	event.Data["fileName"] = media.fileName
	event.Data["fileSize"] = media.declared
	if media.declared > MaxReceivedMediaBytes {
		metadata["error"] = fmt.Sprintf("media exceeds the %d MB receive limit", MaxReceivedMediaBytes>>20)
		return event
	}
	m.mediaDownloads <- struct{}{}
	defer func() { <-m.mediaDownloads }()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	stored, err := m.cacheReceivedMedia(ctx, current, messageID, media)
	if err != nil {
		metadata["error"] = "media download failed"
		return event
	}
	metadata["available"] = true
	metadata["size"] = stored.Size
	metadata["savedAt"] = stored.SavedAt
	event.Data["fileSize"] = stored.Size
	return event
}

func incomingMediaFromMessage(message *waE2E.Message) (incomingMedia, bool) {
	if message == nil {
		return incomingMedia{}, false
	}
	switch {
	case message.GetImageMessage() != nil:
		item := message.GetImageMessage()
		return incomingMedia{download: item, kind: "image", mimeType: cleanReceivedMIME(item.GetMimetype(), "image/jpeg"), declared: item.GetFileLength(), caption: item.GetCaption()}, true
	case message.GetVideoMessage() != nil:
		item := message.GetVideoMessage()
		return incomingMedia{download: item, kind: "video", mimeType: cleanReceivedMIME(item.GetMimetype(), "video/mp4"), declared: item.GetFileLength(), caption: item.GetCaption(), duration: item.GetSeconds()}, true
	case message.GetAudioMessage() != nil:
		item := message.GetAudioMessage()
		return incomingMedia{download: item, kind: "audio", mimeType: cleanReceivedMIME(item.GetMimetype(), "audio/ogg"), declared: item.GetFileLength(), duration: item.GetSeconds(), voice: item.GetPTT()}, true
	case message.GetDocumentMessage() != nil:
		item := message.GetDocumentMessage()
		return incomingMedia{download: item, kind: "document", mimeType: cleanReceivedMIME(item.GetMimetype(), "application/octet-stream"), fileName: item.GetFileName(), declared: item.GetFileLength(), caption: item.GetCaption()}, true
	case message.GetStickerMessage() != nil:
		item := message.GetStickerMessage()
		return incomingMedia{download: item, kind: "sticker", mimeType: cleanReceivedMIME(item.GetMimetype(), "image/webp"), declared: item.GetFileLength(), animated: item.GetIsAnimated()}, true
	default:
		return incomingMedia{}, false
	}
}

func addStructuredMessageMetadata(data map[string]any, message *waE2E.Message) {
	if message == nil {
		return
	}
	if item := message.GetLocationMessage(); item != nil {
		data["type"] = "location"
		data["location"] = map[string]any{
			"latitude": item.GetDegreesLatitude(), "longitude": item.GetDegreesLongitude(),
			"name": item.GetName(), "address": item.GetAddress(), "url": item.GetURL(),
			"isLive": item.GetIsLive(), "accuracy": item.GetAccuracyInMeters(),
		}
		return
	}
	if item := message.GetContactMessage(); item != nil {
		data["type"] = "contact"
		data["contact"] = map[string]any{"displayName": item.GetDisplayName(), "vcard": item.GetVcard()}
		return
	}
	if item := message.GetContactsArrayMessage(); item != nil {
		contacts := make([]map[string]string, 0, len(item.GetContacts()))
		for _, contact := range item.GetContacts() {
			contacts = append(contacts, map[string]string{"displayName": contact.GetDisplayName(), "vcard": contact.GetVcard()})
		}
		data["type"] = "contacts"
		data["contacts"] = contacts
	}
}

func (m *Manager) cacheReceivedMedia(ctx context.Context, current *session, messageID string, media incomingMedia) (receivedMediaMetadata, error) {
	contentPath, metadataPath := m.receivedMediaPaths(current.id, messageID)
	if stored, err := readReceivedMediaMetadata(metadataPath); err == nil {
		if info, statErr := os.Stat(contentPath); statErr == nil && info.Mode().IsRegular() {
			stored.Size = info.Size()
			return stored, nil
		}
	}
	directory := filepath.Dir(contentPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return receivedMediaMetadata{}, fmt.Errorf("create received media directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".download-*")
	if err != nil {
		return receivedMediaMetadata{}, fmt.Errorf("create received media file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := current.client.DownloadToFile(ctx, media.download, temporary); err != nil {
		_ = temporary.Close()
		return receivedMediaMetadata{}, fmt.Errorf("download received media: %w", err)
	}
	info, err := temporary.Stat()
	if err != nil {
		_ = temporary.Close()
		return receivedMediaMetadata{}, fmt.Errorf("inspect received media: %w", err)
	}
	if info.Size() > MaxReceivedMediaBytes {
		_ = temporary.Close()
		return receivedMediaMetadata{}, fmt.Errorf("received media exceeds %d MB", MaxReceivedMediaBytes>>20)
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return receivedMediaMetadata{}, err
	}
	if err := temporary.Close(); err != nil {
		return receivedMediaMetadata{}, err
	}
	if err := os.Rename(temporaryName, contentPath); err != nil {
		return receivedMediaMetadata{}, fmt.Errorf("commit received media: %w", err)
	}
	stored := receivedMediaMetadata{MessageID: messageID, Kind: media.kind, MIMEType: media.mimeType, FileName: media.fileName, Size: info.Size(), SavedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(stored)
	if err != nil {
		return receivedMediaMetadata{}, err
	}
	metadataTemp := metadataPath + ".tmp"
	if err := os.WriteFile(metadataTemp, raw, 0o600); err != nil {
		return receivedMediaMetadata{}, fmt.Errorf("write received media metadata: %w", err)
	}
	if err := os.Rename(metadataTemp, metadataPath); err != nil {
		_ = os.Remove(metadataTemp)
		return receivedMediaMetadata{}, fmt.Errorf("commit received media metadata: %w", err)
	}
	return stored, nil
}

func (m *Manager) OpenReceivedMedia(_ context.Context, instanceID, messageID string) (ReceivedMedia, error) {
	if strings.TrimSpace(instanceID) == "" || strings.TrimSpace(messageID) == "" {
		return ReceivedMedia{}, ErrReceivedMediaNotFound
	}
	contentPath, metadataPath := m.receivedMediaPaths(instanceID, messageID)
	metadata, err := readReceivedMediaMetadata(metadataPath)
	if err != nil || metadata.MessageID != messageID {
		return ReceivedMedia{}, ErrReceivedMediaNotFound
	}
	file, err := os.Open(contentPath)
	if errors.Is(err, os.ErrNotExist) {
		return ReceivedMedia{}, ErrReceivedMediaNotFound
	}
	if err != nil {
		return ReceivedMedia{}, fmt.Errorf("open received media: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return ReceivedMedia{}, fmt.Errorf("inspect received media: %w", err)
	}
	savedAt, _ := time.Parse(time.RFC3339, metadata.SavedAt)
	return ReceivedMedia{File: file, Kind: metadata.Kind, MIMEType: metadata.MIMEType, FileName: metadata.FileName, Size: info.Size(), SavedAt: savedAt}, nil
}

func (m *Manager) PruneReceivedMedia(before time.Time) (int, error) {
	root := filepath.Join(m.dataDirectory, "received-media")
	removed := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".json") {
			metadata, err := readReceivedMediaMetadata(path)
			if err != nil {
				return nil
			}
			savedAt, err := time.Parse(time.RFC3339, metadata.SavedAt)
			if err != nil || !savedAt.Before(before) {
				return nil
			}
			_ = os.Remove(strings.TrimSuffix(path, ".json") + ".bin")
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			removed++
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".download-") || strings.HasSuffix(entry.Name(), ".tmp") {
			info, err := entry.Info()
			if err == nil && info.ModTime().Before(before) {
				_ = os.Remove(path)
			}
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return removed, fmt.Errorf("prune received media: %w", err)
	}
	return removed, nil
}

func readReceivedMediaMetadata(path string) (receivedMediaMetadata, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return receivedMediaMetadata{}, err
	}
	var metadata receivedMediaMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return receivedMediaMetadata{}, err
	}
	return metadata, nil
}

func (m *Manager) receivedMediaPaths(instanceID, messageID string) (string, string) {
	instanceHash := sha256.Sum256([]byte(instanceID))
	messageHash := sha256.Sum256([]byte(instanceID + "\x00" + messageID))
	directory := filepath.Join(m.dataDirectory, "received-media", hex.EncodeToString(instanceHash[:16]))
	base := filepath.Join(directory, hex.EncodeToString(messageHash[:]))
	return base + ".bin", base + ".json"
}

func cleanReceivedMIME(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		return parsed
	}
	return fallback
}

func receivedMediaFileName(messageID, value, mimeType string) string {
	if value = safeFileName(value); value != "" {
		return value
	}
	extension := ""
	if extensions, _ := mime.ExtensionsByType(mimeType); len(extensions) > 0 {
		extension = extensions[0]
	}
	return "media-" + safeIdentifier(messageID) + extension
}

func safeIdentifier(value string) string {
	var builder strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "message"
	}
	return builder.String()
}
