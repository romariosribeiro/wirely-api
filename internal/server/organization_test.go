package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingOrganization struct {
	action, instanceID, first string
	linked, labeled           bool
}

func (o *recordingOrganization) mark(action, id, first string) {
	o.action, o.instanceID, o.first = action, id, first
}
func (o *recordingOrganization) CreateNewsletter(_ context.Context, id, name, _ string) (engine.Newsletter, error) {
	o.mark("create-newsletter", id, name)
	return engine.Newsletter{JID: "1@newsletter", Name: name, State: "active"}, nil
}
func (o *recordingOrganization) GetNewsletter(_ context.Context, id, jid string) (engine.Newsletter, error) {
	o.mark("get-newsletter", id, jid)
	return engine.Newsletter{JID: jid, State: "active"}, nil
}
func (o *recordingOrganization) GetNewsletterByInvite(_ context.Context, id, key string) (engine.Newsletter, error) {
	o.mark("invite", id, key)
	return engine.Newsletter{JID: "1@newsletter", State: "active"}, nil
}
func (o *recordingOrganization) ListNewsletters(_ context.Context, id string) ([]engine.Newsletter, error) {
	o.mark("list", id, "")
	return []engine.Newsletter{}, nil
}
func (o *recordingOrganization) GetNewsletterMessages(_ context.Context, id, jid string, _ int, _ int64) ([]engine.NewsletterMessage, error) {
	o.mark("messages", id, jid)
	return []engine.NewsletterMessage{}, nil
}
func (o *recordingOrganization) SubscribeNewsletter(_ context.Context, id, jid string) error {
	o.mark("subscribe", id, jid)
	return nil
}
func (o *recordingOrganization) SetChatLabel(_ context.Context, id, chat, _ string, labeled bool) (engine.LabelResult, error) {
	o.mark("chat-label", id, chat)
	o.labeled = labeled
	return engine.LabelResult{Target: chat, Labeled: &labeled}, nil
}
func (o *recordingOrganization) SetMessageLabel(_ context.Context, id, chat, messageID, _ string, labeled bool) (engine.LabelResult, error) {
	o.mark("message-label", id, messageID)
	o.labeled = labeled
	return engine.LabelResult{Target: chat, MessageID: messageID, Labeled: &labeled}, nil
}
func (o *recordingOrganization) EditLabel(_ context.Context, id, labelID, _ string, _ int32, _ bool) (engine.LabelResult, error) {
	o.mark("edit-label", id, labelID)
	return engine.LabelResult{LabelID: labelID}, nil
}
func (o *recordingOrganization) CreateCommunity(_ context.Context, id, name string) (engine.Group, error) {
	o.mark("create-community", id, name)
	return engine.Group{JID: "1@g.us", Name: name, IsCommunity: true, Participants: []engine.GroupParticipant{}}, nil
}
func (o *recordingOrganization) UpdateCommunityGroups(_ context.Context, id, community string, _ []string, link bool) (engine.CommunityGroupsResult, error) {
	o.mark("community-groups", id, community)
	o.linked = link
	return engine.CommunityGroupsResult{Success: []string{"2@g.us"}}, nil
}

func TestOrganizationEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Organization")
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingOrganization{}
	app := New(Dependencies{Store: store, Organization: recorder})
	tests := []struct {
		method, path, body, action string
		check                      func() bool
	}{
		{"POST", "/api/newsletters", `{"name":"Canal"}`, "create-newsletter", func() bool { return recorder.first == "Canal" }},
		{"POST", "/api/newsletters/info", `{"jid":"1@newsletter"}`, "get-newsletter", nil},
		{"POST", "/api/newsletters/invite", `{"key":"ABC"}`, "invite", nil},
		{"GET", "/api/newsletters", ``, "list", nil},
		{"POST", "/api/newsletters/messages", `{"jid":"1@newsletter","count":20}`, "messages", nil},
		{"POST", "/api/newsletters/subscribe", `{"jid":"1@newsletter"}`, "subscribe", nil},
		{"POST", "/api/labels/chat", `{"chat":"5511999999999","labelId":"1"}`, "chat-label", func() bool { return recorder.labeled }},
		{"DELETE", "/api/labels/chat", `{"chat":"5511999999999","labelId":"1"}`, "chat-label", func() bool { return !recorder.labeled }},
		{"POST", "/api/labels/message", `{"chat":"5511999999999","messageId":"M1","labelId":"1"}`, "message-label", func() bool { return recorder.labeled }},
		{"DELETE", "/api/labels/message", `{"chat":"5511999999999","messageId":"M1","labelId":"1"}`, "message-label", func() bool { return !recorder.labeled }},
		{"PATCH", "/api/labels/1", `{"name":"Urgente","color":3}`, "edit-label", nil},
		{"POST", "/api/communities", `{"name":"Comunidade"}`, "create-community", nil},
		{"POST", "/api/communities/groups", `{"communityJid":"1@g.us","groupJids":["2@g.us"]}`, "community-groups", func() bool { return recorder.linked }},
		{"DELETE", "/api/communities/groups", `{"communityJid":"1@g.us","groupJids":["2@g.us"]}`, "community-groups", func() bool { return !recorder.linked }},
	}
	for _, test := range tests {
		t.Run(test.method+test.path, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			r.Header.Set("Authorization", "Bearer "+instance.APIToken)
			w := httptest.NewRecorder()
			app.Handler().ServeHTTP(w, r)
			if w.Code < 200 || w.Code >= 300 || recorder.action != test.action || recorder.instanceID != instance.ID || (test.check != nil && !test.check()) {
				t.Fatalf("%s %s: %d %s", test.method, test.path, w.Code, w.Body.String())
			}
		})
	}
}

func TestOrganizationEndpointsRequireBearer(t *testing.T) {
	paths := []string{"/api/newsletters", "/api/newsletters/info", "/api/labels/chat", "/api/communities"}
	app := New(Dependencies{Store: testStore(t), Organization: &recordingOrganization{}})
	for _, path := range paths {
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
}
