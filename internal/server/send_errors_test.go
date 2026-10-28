package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"go.mau.fi/whatsmeow"
)

type failedRecipientSender struct{ err error }

func (s failedRecipientSender) SendText(context.Context, string, string, string) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendMedia(context.Context, string, string, engine.MediaPayload) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendLocation(context.Context, string, string, engine.LocationPayload) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendLiveLocation(context.Context, string, string, engine.LiveLocationPayload, engine.MessageOptions) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendContact(context.Context, string, string, engine.ContactPayload) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendPoll(context.Context, string, string, engine.PollPayload) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendReaction(context.Context, string, string, engine.ReactionPayload) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) ForwardMessage(context.Context, string, string, string, string) (engine.SentMessage, error) {
	return engine.SentMessage{}, s.err
}
func (s failedRecipientSender) SendChatPresence(context.Context, string, string, string) error {
	return s.err
}

func assertRecipientError(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body["error"] != engine.RecipientNotOnWhatsAppMessage ||
		body["code"] != "recipient_not_on_whatsapp" || body["retryable"] != false {
		t.Fatalf("unexpected recipient error: %d %s", response.Code, response.Body.String())
	}
}

func TestSendEndpointsReportUnregisteredRecipient(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Registration errors")
	if err != nil {
		t.Fatal(err)
	}
	// Use the upstream sentinel with multiple wrapping levels, as the real engine does.
	sender := failedRecipientSender{err: fmt.Errorf("send WhatsApp message: %w", fmt.Errorf("no LID: %w", whatsmeow.ErrRecipientNotOnWhatsApp))}
	app := New(Dependencies{Store: store, Sender: sender, Structured: sender, Forwarder: sender})
	for _, test := range []struct{ path, body string }{
		{"/api/send/text", `{"recipient":"5511999999999","message":"Hello"}`},
		{"/api/send/location", `{"recipient":"5511999999999","latitude":0,"longitude":0}`},
		{"/api/send/location/live", `{"recipient":"5511999999999","latitude":0,"longitude":0}`},
		{"/api/send/contact", `{"recipient":"5511999999999","fullName":"Maria","phone":"5511888888888"}`},
		{"/api/send/poll", `{"recipient":"5511999999999","question":"Choose","choices":["A","B"],"maxAnswer":1}`},
		{"/api/send/reaction", `{"recipient":"5511999999999","messageId":"ABC123","reaction":"👍","fromMe":true}`},
		{"/api/messages/forward", `{"chat":"5511888888888","messageId":"ABC123","recipient":"5511999999999"}`},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer "+instance.APIToken)
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			assertRecipientError(t, response)
		})
	}
	for _, kind := range []string{"image", "video", "audio", "document", "sticker"} {
		t.Run(kind, func(t *testing.T) {
			mime := map[string]string{"image": "image/png", "video": "video/mp4", "audio": "audio/ogg", "document": "application/pdf", "sticker": "image/webp"}[kind]
			data := []byte("file bytes")
			if kind == "image" {
				data = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'H', 'D', 'R'}
			} else if kind == "sticker" {
				data = []byte{'R', 'I', 'F', 'F', 12, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
			}
			request := multipartRequest(t, "/api/send/media", instance.APIToken, "5511999999999", "file", mime, data, map[string]string{"type": kind})
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			assertRecipientError(t, response)
		})
	}
	t.Run("presence before sending", func(t *testing.T) {
		app := New(Dependencies{Store: store, Sender: &recordingSender{}, Presence: sender})
		request := httptest.NewRequest(http.MethodPost, "/api/send/text", strings.NewReader(`{"recipient":"5511999999999","message":"Hello","options":{"presence":"composing","delay":60000}}`))
		request.Header.Set("Authorization", "Bearer "+instance.APIToken)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		assertRecipientError(t, response)
	})
	t.Run("panel reply", func(t *testing.T) {
		app := New(Dependencies{Store: store, ChatSender: &fakeChatSender{err: sender.err}})
		cookie := authenticatedCookie(t, store, app.Handler())
		request := httptest.NewRequest(http.MethodPost, "/api/instances/"+instance.ID+"/chats/5511999999999@s.whatsapp.net/messages", strings.NewReader(`{"message":"Hello"}`))
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		assertRecipientError(t, response)
	})
}

func TestWriteSendErrorPreservesOtherErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{nil, http.StatusOK},
		{fmt.Errorf("wrapped: %w", engine.ErrNotConnected), http.StatusConflict},
		{context.DeadlineExceeded, http.StatusUnprocessableEntity},
		{errors.New("temporary lookup failure"), http.StatusUnprocessableEntity},
		{errors.New("recipient is not on WhatsApp"), http.StatusUnprocessableEntity},
	} {
		response := httptest.NewRecorder()
		if wrote := writeSendError(response, test.err); wrote != (test.err != nil) || response.Code != test.status {
			t.Fatalf("error=%v: status=%d wrote=%v", test.err, response.Code, wrote)
		}
		if test.err != nil {
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["error"] != test.err.Error() || len(body) != 1 {
				t.Fatalf("unrelated error was reclassified: %s (%v)", response.Body.String(), err)
			}
		}
	}
}
