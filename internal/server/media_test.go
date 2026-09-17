package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingSender struct {
	media                       engine.MediaPayload
	recipient, instanceID, text string
	calls                       int
}

func (s *recordingSender) SendText(_ context.Context, id, recipient, text string) (engine.SentMessage, error) {
	s.calls++
	s.instanceID = id
	s.recipient = recipient
	s.text = text
	return engine.SentMessage{ID: "msg_text", Recipient: recipient, Timestamp: "2026-09-16T20:00:00Z", Type: "text"}, nil
}

func (s *recordingSender) SendMedia(_ context.Context, id, recipient string, media engine.MediaPayload) (engine.SentMessage, error) {
	s.calls++
	s.instanceID = id
	s.recipient = recipient
	s.media = media
	return engine.SentMessage{ID: "msg_media", Recipient: recipient, Timestamp: "2026-09-16T20:00:00Z", Type: string(media.Kind)}, nil
}

func multipartRequest(t *testing.T, endpoint, token, recipient, filename, contentType string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("recipient", recipient)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, endpoint, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestPublicMessageEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Messages")
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	presence := &recordingPresenceSender{}
	app := New(Dependencies{Store: store, Sender: sender, Presence: presence})

	textBody, _ := json.Marshal(map[string]any{"recipient": "5511999999999", "message": "Hello", "options": map[string]any{"presence": "composing", "delay": 1}})
	text := httptest.NewRequest(http.MethodPost, "/api/send/text", bytes.NewReader(textBody))
	text.Header.Set("Authorization", "Bearer "+instance.APIToken)
	textResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(textResponse, text)
	if textResponse.Code != http.StatusCreated || sender.text != "Hello" || !strings.Contains(textResponse.Body.String(), `"type":"text"`) {
		t.Fatalf("text failed: %d %s", textResponse.Code, textResponse.Body.String())
	}

	cases := []struct {
		kind, filename, contentType string
		data                        []byte
		fields                      map[string]string
		check                       func(engine.MediaPayload) bool
	}{
		{"image", "photo.png", "application/octet-stream", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}, map[string]string{"caption": "Photo"},
			func(media engine.MediaPayload) bool {
				return media.Kind == engine.MediaImage && media.MIMEType == "image/png" && media.Caption == "Photo"
			}},
		{"video", "clip.mp4", "video/mp4", []byte("video-data"), map[string]string{"caption": "Clip"},
			func(media engine.MediaPayload) bool {
				return media.Kind == engine.MediaVideo && media.MIMEType == "video/mp4" && media.Caption == "Clip"
			}},
		{"audio", "voice.ogg", "audio/ogg", []byte("OggSdata"), map[string]string{"voice": "true"},
			func(media engine.MediaPayload) bool { return media.Kind == engine.MediaAudio && media.Voice }},
		{"document", "invoice.pdf", "application/pdf", []byte("%PDF-1.7"), map[string]string{"caption": "Invoice"},
			func(media engine.MediaPayload) bool {
				return media.Kind == engine.MediaDocument && media.FileName == "invoice.pdf"
			}},
		{"sticker", "sticker.webp", "image/webp", []byte{'R', 'I', 'F', 'F', 12, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}, nil,
			func(media engine.MediaPayload) bool {
				return media.Kind == engine.MediaSticker && media.MIMEType == "image/webp"
			}},
	}
	for _, test := range cases {
		t.Run(test.kind, func(t *testing.T) {
			fields := map[string]string{"type": test.kind}
			presenceType := map[bool]string{true: "recording", false: "composing"}[test.kind == "audio"]
			fields["options"] = `{"presence":"` + presenceType + `","delay":1}`
			for key, value := range test.fields {
				fields[key] = value
			}
			request := multipartRequest(t, "/api/send/media", instance.APIToken, "5511999999999", test.filename, test.contentType, test.data, fields)
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusCreated || !test.check(sender.media) || !strings.Contains(response.Body.String(), `"type":"`+test.kind+`"`) {
				t.Fatalf("%s failed: %d %s %#v", test.kind, response.Code, response.Body.String(), sender.media)
			}
		})
	}
	if sender.instanceID != instance.ID || sender.calls != 6 {
		t.Fatalf("wrong sender calls: %#v", sender)
	}
	if len(presence.states) != 12 || presence.states[0] != "composing" || presence.states[1] != "paused" {
		t.Fatalf("unexpected presence sequence: %#v", presence.states)
	}
}

func TestUnifiedMediaEndpointValidatesAuthenticationTypeAndPayload(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Validation")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store, Sender: &recordingSender{}})

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/send/media", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, unauthorized)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatal("unified media endpoint must require bearer")
	}

	missingType := multipartRequest(t, "/api/send/media", instance.APIToken, "+5511999999999", "photo.png", "image/png", []byte("png"), nil)
	missingTypeResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(missingTypeResponse, missingType)
	if missingTypeResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(missingTypeResponse.Body.String(), "type must be") {
		t.Fatalf("missing type accepted: %d %s", missingTypeResponse.Code, missingTypeResponse.Body.String())
	}

	wrong := multipartRequest(t, "/api/send/media", instance.APIToken, "+5511999999999", "note.txt", "text/plain", []byte("plain text"), map[string]string{"type": "image"})
	wrongResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong media accepted: %d %s", wrongResponse.Code, wrongResponse.Body.String())
	}

	for _, oldEndpoint := range []string{"/api/send/image", "/api/send/audio", "/api/send/document"} {
		legacy := httptest.NewRequest(http.MethodPost, oldEndpoint, nil)
		legacyResponse := httptest.NewRecorder()
		app.Handler().ServeHTTP(legacyResponse, legacy)
		if legacyResponse.Code != http.StatusNotFound {
			t.Fatalf("legacy endpoint %s still exists: %d", oldEndpoint, legacyResponse.Code)
		}
	}
}
