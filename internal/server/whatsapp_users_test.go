package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingWhatsAppUserManager struct {
	instanceID string
	number     string
	preview    bool
	blocked    bool
	numbers    []string
}

func (manager *recordingWhatsAppUserManager) GetUserAvatar(_ context.Context, id, number string, preview bool) (engine.UserAvatar, error) {
	manager.instanceID, manager.number, manager.preview = id, number, preview
	return engine.UserAvatar{JID: number + "@s.whatsapp.net", URL: "https://example.test/avatar.jpg"}, nil
}

func (manager *recordingWhatsAppUserManager) GetBlocklist(_ context.Context, id string) ([]engine.BlockedUser, error) {
	manager.instanceID = id
	return []engine.BlockedUser{{JID: "5511888888888@s.whatsapp.net", Phone: "5511888888888"}}, nil
}

func (manager *recordingWhatsAppUserManager) SetContactBlocked(_ context.Context, id, number string, blocked bool) ([]engine.BlockedUser, error) {
	manager.instanceID, manager.number, manager.blocked = id, number, blocked
	return []engine.BlockedUser{}, nil
}

func (manager *recordingWhatsAppUserManager) GetUsers(_ context.Context, id string, numbers []string) ([]engine.UserDetails, error) {
	manager.instanceID, manager.numbers = id, append([]string(nil), numbers...)
	return []engine.UserDetails{{JID: numbers[0] + "@s.whatsapp.net", Phone: numbers[0]}}, nil
}

func TestWhatsAppUserEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Users")
	if err != nil {
		t.Fatal(err)
	}
	manager := &recordingWhatsAppUserManager{}
	contacts := fakeContactProvider{items: []engine.Contact{{JID: "5511999999999@s.whatsapp.net", Name: "Maria"}}}
	app := New(Dependencies{Store: store, WhatsAppUsers: manager, Contacts: contacts})
	tests := []struct {
		method, path, body string
		check              func() bool
	}{
		{http.MethodPost, "/api/user/avatar", `{"number":"5511999999999","preview":true}`, func() bool { return manager.number == "5511999999999" && manager.preview }},
		{http.MethodPost, "/api/user/block", `{"number":"5511888888888"}`, func() bool { return manager.number == "5511888888888" && manager.blocked }},
		{http.MethodGet, "/api/user/blocklist", "", func() bool { return manager.instanceID == instance.ID }},
		{http.MethodGet, "/api/user/contacts", "", func() bool { return true }},
		{http.MethodPost, "/api/user/info", `{"number":["5511999999999"]}`, func() bool { return len(manager.numbers) == 1 }},
		{http.MethodPost, "/api/user/unblock", `{"number":"5511888888888"}`, func() bool { return manager.number == "5511888888888" && !manager.blocked }},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
		request.Header.Set("Authorization", "Bearer "+instance.APIToken)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || !test.check() || !strings.Contains(response.Body.String(), "{") {
			t.Fatalf("%s %s: %d %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestWhatsAppUserEndpointsRequireBearer(t *testing.T) {
	response := httptest.NewRecorder()
	New(Dependencies{Store: testStore(t), WhatsAppUsers: &recordingWhatsAppUserManager{}}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/user/blocklist", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
}
