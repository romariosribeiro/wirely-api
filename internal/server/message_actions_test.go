package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingMessageActions struct {
	instanceID, chat, messageID, message, participant string
	messageIDs                                        []string
	archived, pinned                                  bool
	duration                                          time.Duration
}

func (m *recordingMessageActions) DeleteMessage(_ context.Context, id, chat, messageID, participant string) (engine.MessageActionResult, error) {
	m.instanceID, m.chat, m.messageID, m.participant = id, chat, messageID, participant
	return engine.MessageActionResult{MessageID: messageID, Chat: chat, Status: "deleted"}, nil
}
func (m *recordingMessageActions) EditMessage(_ context.Context, id, chat, messageID, message string) (engine.MessageActionResult, error) {
	m.instanceID, m.chat, m.messageID, m.message = id, chat, messageID, message
	return engine.MessageActionResult{MessageID: messageID, Chat: chat, Status: "edited"}, nil
}
func (m *recordingMessageActions) MarkMessagesRead(_ context.Context, id, chat string, messageIDs []string, participant string) (engine.MessageActionResult, error) {
	m.instanceID, m.chat, m.messageIDs, m.participant = id, chat, messageIDs, participant
	return engine.MessageActionResult{MessageID: messageIDs[0], Chat: chat, Status: "read"}, nil
}
func (m *recordingMessageActions) ArchiveChat(_ context.Context, id, chat string, archived bool) (engine.ChatActionResult, error) {
	m.instanceID, m.chat, m.archived = id, chat, archived
	return engine.ChatActionResult{Chat: chat, Action: "archive", Status: "archived"}, nil
}
func (m *recordingMessageActions) MuteChat(_ context.Context, id, chat string, duration time.Duration) (engine.ChatActionResult, error) {
	m.instanceID, m.chat, m.duration = id, chat, duration
	return engine.ChatActionResult{Chat: chat, Action: "mute", Status: "muted"}, nil
}
func (m *recordingMessageActions) PinChat(_ context.Context, id, chat string, pinned bool) (engine.ChatActionResult, error) {
	m.instanceID, m.chat, m.pinned = id, chat, pinned
	return engine.ChatActionResult{Chat: chat, Action: "pin", Status: "pinned"}, nil
}

func TestMessageAndChatActionEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Actions")
	if err != nil {
		t.Fatal(err)
	}
	actions := &recordingMessageActions{}
	app := New(Dependencies{Store: store, MessageActions: actions})
	tests := []struct {
		path, body, status string
		check              func() bool
	}{
		{"/api/messages/delete", `{"chat":"5511999999999","messageId":"MSG-1"}`, "deleted", func() bool { return actions.messageID == "MSG-1" }},
		{"/api/messages/edit", `{"chat":"5511999999999","messageId":"MSG-1","message":"novo"}`, "edited", func() bool { return actions.message == "novo" }},
		{"/api/messages/read", `{"chat":"5511999999999","messageIds":["MSG-1","MSG-2"]}`, "read", func() bool { return len(actions.messageIDs) == 2 }},
		{"/api/chats/archive", `{"chat":"5511999999999"}`, "archived", func() bool { return actions.archived }},
		{"/api/chats/mute", `{"chat":"5511999999999","durationSeconds":3600}`, "muted", func() bool { return actions.duration == time.Hour }},
		{"/api/chats/pin", `{"chat":"5511999999999"}`, "pinned", func() bool { return actions.pinned }},
		{"/api/chats/unpin", `{"chat":"5511999999999"}`, "pinned", func() bool { return !actions.pinned }},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer "+instance.APIToken)
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"`+test.status+`"`) || actions.instanceID != instance.ID || !test.check() {
			t.Fatalf("%s failed: %d %s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestMessageStatusIsScopedByBearer(t *testing.T) {
	store := testStore(t)
	first, _ := store.CreateInstance(context.Background(), "First")
	second, _ := store.CreateInstance(context.Background(), "Second")
	_, err := store.SaveActivityEvent(context.Background(), "evt-status", first.ID, "message.sent", "2026-09-17T10:00:00Z", map[string]any{"id": "MSG-1", "chat": "5511999999999@s.whatsapp.net"})
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	request := func(token string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/status", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		app.Handler().ServeHTTP(response, r)
		return response
	}
	if response := request(first.APIToken); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"sent"`) {
		t.Fatalf("owner could not read status: %d %s", response.Code, response.Body.String())
	}
	if response := request(second.APIToken); response.Code != http.StatusNotFound {
		t.Fatalf("other instance accessed status: %d %s", response.Code, response.Body.String())
	}
}

func TestMessageActionsRequireBearer(t *testing.T) {
	response := httptest.NewRecorder()
	New(Dependencies{Store: testStore(t), MessageActions: &recordingMessageActions{}}).Handler().ServeHTTP(response,
		httptest.NewRequest(http.MethodPost, "/api/messages/delete", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
