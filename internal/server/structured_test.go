package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingStructuredSender struct {
	instanceID string
	recipient  string
	kind       string
	location   engine.LocationPayload
	contact    engine.ContactPayload
	poll       engine.PollPayload
	reaction   engine.ReactionPayload
}

func structuredResult(kind, recipient string) engine.SentMessage {
	return engine.SentMessage{ID: "msg_" + kind, Recipient: recipient, Timestamp: "2026-09-16T20:00:00Z", Type: kind}
}

func (s *recordingStructuredSender) SendLocation(_ context.Context, id, recipient string, payload engine.LocationPayload) (engine.SentMessage, error) {
	s.instanceID, s.recipient, s.kind, s.location = id, recipient, "location", payload
	return structuredResult(s.kind, recipient), nil
}

func (s *recordingStructuredSender) SendContact(_ context.Context, id, recipient string, payload engine.ContactPayload) (engine.SentMessage, error) {
	s.instanceID, s.recipient, s.kind, s.contact = id, recipient, "contact", payload
	return structuredResult(s.kind, recipient), nil
}

func (s *recordingStructuredSender) SendPoll(_ context.Context, id, recipient string, payload engine.PollPayload) (engine.SentMessage, error) {
	s.instanceID, s.recipient, s.kind, s.poll = id, recipient, "poll", payload
	return structuredResult(s.kind, recipient), nil
}

func (s *recordingStructuredSender) SendReaction(_ context.Context, id, recipient string, payload engine.ReactionPayload) (engine.SentMessage, error) {
	s.instanceID, s.recipient, s.kind, s.reaction = id, recipient, "reaction", payload
	return structuredResult(s.kind, recipient), nil
}

func TestStructuredMessageEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Structured")
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingStructuredSender{}
	presence := &recordingPresenceSender{}
	app := New(Dependencies{Store: store, Structured: sender, Presence: presence})
	tests := []struct {
		path  string
		body  map[string]any
		kind  string
		check func() bool
	}{
		{"/api/send/location", map[string]any{"recipient": "5511999999999", "latitude": -23.5505, "longitude": -46.6333, "name": "Sé"}, "location", func() bool { return sender.location.Name == "Sé" }},
		{"/api/send/contact", map[string]any{"recipient": "5511999999999", "fullName": "Maria", "organization": "Wirely", "phone": "5511888888888"}, "contact", func() bool { return sender.contact.Phone == "5511888888888" }},
		{"/api/send/poll", map[string]any{"recipient": "5511999999999", "question": "Escolha", "choices": []string{"A", "B"}, "maxAnswer": 1}, "poll", func() bool { return len(sender.poll.Options) == 2 }},
		{"/api/send/reaction", map[string]any{"recipient": "5511999999999", "messageId": "ABC123", "reaction": "👍", "fromMe": true}, "reaction", func() bool { return sender.reaction.FromMe && sender.reaction.MessageID == "ABC123" }},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			test.body["options"] = map[string]any{"presence": "composing", "delay": 1}
			body, _ := json.Marshal(test.body)
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+instance.APIToken)
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusCreated || sender.instanceID != instance.ID || sender.recipient != "5511999999999" ||
				sender.kind != test.kind || !test.check() || !strings.Contains(response.Body.String(), `"type":"`+test.kind+`"`) {
				t.Fatalf("%s failed: %d %s %#v", test.kind, response.Code, response.Body.String(), sender)
			}
		})
	}
	if len(presence.states) != 8 {
		t.Fatalf("unexpected presence sequence: %#v", presence.states)
	}
}

func TestStructuredMessagesValidateAuthAvailabilityAndPayload(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Validation")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/send/location", strings.NewReader(`{}`))
	unauthorizedResponse := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorizedResponse.Code)
	}

	unavailable := httptest.NewRequest(http.MethodPost, "/api/send/location", strings.NewReader(`{"recipient":"+5511999999999","latitude":0,"longitude":0}`))
	unavailable.Header.Set("Authorization", "Bearer "+instance.APIToken)
	unavailableResponse := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(unavailableResponse, unavailable)
	if unavailableResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", unavailableResponse.Code)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/api/send/poll", strings.NewReader(`{"recipient":"+5511999999999","question":"Q","choices":["A"]}`))
	invalid.Header.Set("Authorization", "Bearer "+instance.APIToken)
	invalidResponse := httptest.NewRecorder()
	New(Dependencies{Store: store, Structured: &recordingStructuredSender{}}).Handler().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidResponse.Body.String(), "between 2 and 12") {
		t.Fatalf("invalid poll accepted: %d %s", invalidResponse.Code, invalidResponse.Body.String())
	}
}
