package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type fakeContactProvider struct {
	items []engine.Contact
	err   error
}

func (provider fakeContactProvider) Contacts(context.Context, string) ([]engine.Contact, error) {
	return append([]engine.Contact(nil), provider.items...), provider.err
}

type fakeChatSender struct {
	instanceID string
	chat       string
	message    string
	err        error
}

func (sender *fakeChatSender) SendChatText(_ context.Context, instanceID, chat, message string) (engine.SentMessage, error) {
	sender.instanceID, sender.chat, sender.message = instanceID, chat, message
	if sender.err != nil {
		return engine.SentMessage{}, sender.err
	}
	return engine.SentMessage{ID: "msg_admin", Recipient: chat, Timestamp: "2026-09-16T20:00:00Z", Type: "text"}, nil
}

func TestInboxAPIListsContactsChatsAndMessages(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	instance, err := store.CreateInstance(ctx, "Inbox API")
	if err != nil {
		t.Fatal(err)
	}
	chat := "5511999999999@s.whatsapp.net"
	_, err = store.SaveActivityEvent(ctx, "evt_inbox_api", instance.ID, "message.received", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{
		"id": "msg_inbox_api", "chat": chat, "from": chat, "fromMe": false, "isGroup": false,
		"pushName": "Nome antigo", "type": "text", "text": "Olá pelo inbox",
	})
	if err != nil {
		t.Fatal(err)
	}
	contacts := fakeContactProvider{items: []engine.Contact{{JID: chat, Name: "Cliente Premium", Phone: "+5511999999999"}}}
	app := New(Dependencies{Store: store, Contacts: contacts})
	cookie := authenticatedCookie(t, store, app.Handler())

	contactRequest := httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/contacts?search=premium", nil)
	contactRequest.AddCookie(cookie)
	contactResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(contactResponse, contactRequest)
	if contactResponse.Code != http.StatusOK || !strings.Contains(contactResponse.Body.String(), "Cliente Premium") {
		t.Fatalf("unexpected contacts response: %d %s", contactResponse.Code, contactResponse.Body.String())
	}

	chatRequest := httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/chats?search=premium", nil)
	chatRequest.AddCookie(cookie)
	chatResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(chatResponse, chatRequest)
	if chatResponse.Code != http.StatusOK || !strings.Contains(chatResponse.Body.String(), `"name":"Cliente Premium"`) || !strings.Contains(chatResponse.Body.String(), `"unreadCount":1`) {
		t.Fatalf("unexpected chats response: %d %s", chatResponse.Code, chatResponse.Body.String())
	}

	messageRequest := httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/chats/"+chat+"/messages", nil)
	messageRequest.AddCookie(cookie)
	messageResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(messageResponse, messageRequest)
	if messageResponse.Code != http.StatusOK || !strings.Contains(messageResponse.Body.String(), "Olá pelo inbox") {
		t.Fatalf("unexpected messages response: %d %s", messageResponse.Code, messageResponse.Body.String())
	}

	readRequest := httptest.NewRequest(http.MethodPost, "/api/instances/"+instance.ID+"/chats/"+chat+"/read", nil)
	readRequest.AddCookie(cookie)
	readResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(readResponse, readRequest)
	if readResponse.Code != http.StatusNoContent {
		t.Fatalf("mark read failed: %d %s", readResponse.Code, readResponse.Body.String())
	}
	chatResponse = httptest.NewRecorder()
	chatRequest = httptest.NewRequest(http.MethodGet, "/api/instances/"+instance.ID+"/chats", nil)
	chatRequest.AddCookie(cookie)
	app.Handler().ServeHTTP(chatResponse, chatRequest)
	if !strings.Contains(chatResponse.Body.String(), `"unreadCount":0`) {
		t.Fatalf("chat remained unread: %s", chatResponse.Body.String())
	}
}

func TestInboxAPISendsReplyThroughChatSender(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Reply API")
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeChatSender{}
	app := New(Dependencies{Store: store, ChatSender: sender})
	cookie := authenticatedCookie(t, store, app.Handler())
	chat := "120363000000@g.us"
	body, _ := json.Marshal(map[string]string{"message": "Resposta pelo painel"})
	request := httptest.NewRequest(http.MethodPost, "/api/instances/"+instance.ID+"/chats/"+chat+"/messages", bytes.NewReader(body))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("send reply failed: %d %s", response.Code, response.Body.String())
	}
	if sender.instanceID != instance.ID || sender.chat != chat || sender.message != "Resposta pelo painel" {
		t.Fatalf("unexpected sender call: %#v", sender)
	}
}

func TestInboxRoutesRequireAuthentication(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodGet, "/api/instances/example/chats", nil)
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
