package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestIncomingMediaMetadata(t *testing.T) {
	message := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		Mimetype: proto.String("audio/ogg; codecs=opus"), FileLength: proto.Uint64(2048),
		Seconds: proto.Uint32(9), PTT: proto.Bool(true),
	}}
	media, ok := incomingMediaFromMessage(message)
	if !ok || media.kind != "audio" || media.mimeType != "audio/ogg" || media.declared != 2048 || media.duration != 9 || !media.voice {
		t.Fatalf("unexpected audio metadata: %#v", media)
	}

	document, ok := incomingMediaFromMessage(&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		Mimetype: proto.String("application/pdf"), FileName: proto.String("../invoice.pdf"),
		FileLength: proto.Uint64(42), Caption: proto.String("Fatura"),
	}})
	if !ok || receivedMediaFileName("MSG-1", document.fileName, document.mimeType) != "invoice.pdf" || document.caption != "Fatura" {
		t.Fatalf("unexpected document metadata: %#v", document)
	}
}

func TestStructuredIncomingMessageMetadata(t *testing.T) {
	data := map[string]any{}
	addStructuredMessageMetadata(data, &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(-23.5505), DegreesLongitude: proto.Float64(-46.6333),
		Name: proto.String("Praça da Sé"), Address: proto.String("São Paulo"),
	}})
	location, ok := data["location"].(map[string]any)
	if !ok || data["type"] != "location" || location["name"] != "Praça da Sé" {
		t.Fatalf("unexpected location metadata: %#v", data)
	}

	data = map[string]any{}
	addStructuredMessageMetadata(data, &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: proto.String("Maria"), Vcard: proto.String("BEGIN:VCARD"),
	}})
	contact, ok := data["contact"].(map[string]any)
	if !ok || data["type"] != "contact" || contact["displayName"] != "Maria" {
		t.Fatalf("unexpected contact metadata: %#v", data)
	}
}

func TestOpenReceivedMediaIsScopedByInstance(t *testing.T) {
	manager := &Manager{dataDirectory: t.TempDir()}
	contentPath, metadataPath := manager.receivedMediaPaths("instance-a", "message-1")
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := []byte(`{"messageId":"message-1","type":"image","mimetype":"image/png","fileName":"photo.png","size":5,"savedAt":"2026-09-17T12:00:00Z"}`)
	if err := os.WriteFile(metadataPath, metadata, 0o600); err != nil {
		t.Fatal(err)
	}

	opened, err := manager.OpenReceivedMedia(context.Background(), "instance-a", "message-1")
	if err != nil {
		t.Fatal(err)
	}
	defer opened.File.Close()
	if opened.Size != 5 || opened.FileName != "photo.png" || opened.SavedAt.IsZero() {
		t.Fatalf("unexpected received media: %#v", opened)
	}
	if _, err := manager.OpenReceivedMedia(context.Background(), "instance-b", "message-1"); !errors.Is(err, ErrReceivedMediaNotFound) {
		t.Fatalf("media escaped instance scope: %v", err)
	}
	if strings.Contains(contentPath, "instance-a") || strings.Contains(contentPath, "message-1") {
		t.Fatalf("raw identifiers leaked into storage path: %s", contentPath)
	}
}

func TestPruneReceivedMedia(t *testing.T) {
	manager := &Manager{dataDirectory: t.TempDir()}
	writeStoredMedia := func(messageID string, savedAt time.Time) (string, string) {
		t.Helper()
		contentPath, metadataPath := manager.receivedMediaPaths("instance-a", messageID)
		if err := os.MkdirAll(filepath.Dir(contentPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contentPath, []byte(messageID), 0o600); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(receivedMediaMetadata{MessageID: messageID, Kind: "image", MIMEType: "image/png",
			FileName: messageID + ".png", Size: int64(len(messageID)), SavedAt: savedAt.UTC().Format(time.RFC3339)})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metadataPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		return contentPath, metadataPath
	}
	now := time.Now().UTC()
	oldContent, oldMetadata := writeStoredMedia("old-message", now.Add(-31*24*time.Hour))
	newContent, newMetadata := writeStoredMedia("new-message", now.Add(-29*24*time.Hour))

	removed, err := manager.PruneReceivedMedia(now.Add(-30 * 24 * time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("unexpected prune result: %d %v", removed, err)
	}
	for _, path := range []string{oldContent, oldMetadata} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old media was not removed: %s", path)
		}
	}
	for _, path := range []string{newContent, newMetadata} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recent media was removed: %s: %v", path, err)
		}
	}
}
