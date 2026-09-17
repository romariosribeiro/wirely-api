package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingGroupManager struct {
	instanceID   string
	groupJID     string
	name         string
	action       string
	participants []string
	reset        bool
	code         string
}

func sampleGroup() engine.Group {
	return engine.Group{JID: "120363000000000000@g.us", Name: "Wirely", ParticipantCount: 2, Participants: []engine.GroupParticipant{}}
}

func (g *recordingGroupManager) ListGroups(_ context.Context, id string) ([]engine.Group, error) {
	g.instanceID = id
	return []engine.Group{sampleGroup()}, nil
}
func (g *recordingGroupManager) GetGroup(_ context.Context, id, jid string) (engine.Group, error) {
	g.instanceID, g.groupJID = id, jid
	return sampleGroup(), nil
}
func (g *recordingGroupManager) CreateGroup(_ context.Context, id, name string, participants []string) (engine.Group, error) {
	g.instanceID, g.name, g.participants = id, name, participants
	return sampleGroup(), nil
}
func (g *recordingGroupManager) SetGroupName(_ context.Context, id, jid, name string) error {
	g.instanceID, g.groupJID, g.name = id, jid, name
	return nil
}
func (g *recordingGroupManager) UpdateGroupParticipants(_ context.Context, id, jid, action string, participants []string) ([]engine.GroupParticipant, error) {
	g.instanceID, g.groupJID, g.action, g.participants = id, jid, action, participants
	return []engine.GroupParticipant{{JID: "5511999999999@s.whatsapp.net", IsAdmin: action == "promote"}}, nil
}
func (g *recordingGroupManager) GroupInviteLink(_ context.Context, id, jid string, reset bool) (string, error) {
	g.instanceID, g.groupJID, g.reset = id, jid, reset
	return "https://chat.whatsapp.com/code", nil
}
func (g *recordingGroupManager) JoinGroup(_ context.Context, id, code string) (string, error) {
	g.instanceID, g.code = id, code
	return sampleGroup().JID, nil
}

func groupRequest(t *testing.T, method, path, token string, payload any) *http.Request {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestGroupEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Groups")
	if err != nil {
		t.Fatal(err)
	}
	groups := &recordingGroupManager{}
	app := New(Dependencies{Store: store, Groups: groups})
	jid := "120363000000000000@g.us"
	tests := []struct {
		method string
		path   string
		body   any
		status int
		check  func() bool
	}{
		{http.MethodGet, "/api/groups", nil, 200, func() bool { return groups.instanceID == instance.ID }},
		{http.MethodPost, "/api/groups", map[string]any{"name": "Wirely", "participants": []string{"+5511999999999"}}, 201, func() bool { return groups.name == "Wirely" && len(groups.participants) == 1 }},
		{http.MethodGet, "/api/groups/" + jid, nil, 200, func() bool { return groups.groupJID == jid }},
		{http.MethodPatch, "/api/groups/" + jid, map[string]any{"name": "Novo nome"}, 204, func() bool { return groups.name == "Novo nome" }},
		{http.MethodPost, "/api/groups/" + jid + "/participants", map[string]any{"action": "promote", "participants": []string{"+5511999999999"}}, 200, func() bool { return groups.action == "promote" }},
		{http.MethodGet, "/api/groups/" + jid + "/invite", nil, 200, func() bool { return !groups.reset }},
		{http.MethodPost, "/api/groups/" + jid + "/invite", nil, 200, func() bool { return groups.reset }},
		{http.MethodPost, "/api/groups/join", map[string]any{"code": "https://chat.whatsapp.com/code"}, 200, func() bool { return groups.code != "" }},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, groupRequest(t, test.method, test.path, instance.APIToken, test.body))
		if response.Code != test.status || !test.check() {
			t.Fatalf("%s %s: got %d %s state=%#v", test.method, test.path, response.Code, response.Body.String(), groups)
		}
	}
}

func TestGroupsRequireBearerAndEngine(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Groups")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	unauthorized := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/groups", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}
	unavailable := httptest.NewRecorder()
	app.Handler().ServeHTTP(unavailable, groupRequest(t, http.MethodGet, "/api/groups", instance.APIToken, nil))
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", unavailable.Code)
	}
}
