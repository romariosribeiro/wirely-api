package engine

import (
	"strings"
	"testing"
)

func TestValidateLocation(t *testing.T) {
	payload := LocationPayload{Latitude: -23.5505, Longitude: -46.6333, Name: "  São Paulo  ", Address: "  Praça da Sé  "}
	if err := ValidateLocation(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Name != "São Paulo" || payload.Address != "Praça da Sé" {
		t.Fatalf("location was not normalized: %#v", payload)
	}
	for _, invalid := range []LocationPayload{{Latitude: -91}, {Latitude: 91}, {Longitude: -181}, {Longitude: 181}} {
		if err := ValidateLocation(&invalid); err == nil {
			t.Fatalf("invalid coordinates accepted: %#v", invalid)
		}
	}
}

func TestValidateContactAndVCard(t *testing.T) {
	payload := ContactPayload{FullName: "  Maria; Silva\nEquipe  ", Organization: " Wirely, Inc. ", Phone: "+55 (11) 99999-9999"}
	if err := ValidateContact(&payload); err != nil {
		t.Fatal(err)
	}
	vcard := contactVCard(payload)
	if payload.Phone != "5511999999999" || !strings.Contains(vcard, "FN:Maria\\; Silva\\nEquipe") ||
		!strings.Contains(vcard, "ORG:Wirely\\, Inc.") || !strings.Contains(vcard, "WAID=5511999999999") {
		t.Fatalf("unexpected vCard: %q", vcard)
	}
	if strings.Contains(vcard, "\nEquipe\r\nORG") {
		t.Fatal("raw newline was not escaped")
	}
}

func TestValidatePoll(t *testing.T) {
	payload := PollPayload{Question: "  Qual opção? ", Options: []string{" A ", "B"}, MaxAnswer: 0}
	if err := ValidatePoll(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Question != "Qual opção?" || payload.Options[0] != "A" || payload.MaxAnswer != 1 {
		t.Fatalf("poll was not normalized: %#v", payload)
	}
	for _, invalid := range []PollPayload{
		{Question: "", Options: []string{"A", "B"}, MaxAnswer: 1},
		{Question: "Q", Options: []string{"A"}, MaxAnswer: 1},
		{Question: "Q", Options: []string{"A", " a "}, MaxAnswer: 1},
		{Question: "Q", Options: []string{"A", "B"}, MaxAnswer: 3},
	} {
		if err := ValidatePoll(&invalid); err == nil {
			t.Fatalf("invalid poll accepted: %#v", invalid)
		}
	}
}

func TestValidateReactionAllowsEmptyToRemove(t *testing.T) {
	payload := ReactionPayload{MessageID: " message-id ", Reaction: ""}
	if err := ValidateReaction(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.MessageID != "message-id" {
		t.Fatalf("message ID was not normalized: %q", payload.MessageID)
	}
	if err := ValidateReaction(&ReactionPayload{Reaction: "👍"}); err == nil {
		t.Fatal("missing message ID was accepted")
	}
}

func TestParseMessageRecipientAcceptsPhoneAndGroupJID(t *testing.T) {
	phone, display, err := parseMessageRecipient("+55 11 99999-9999")
	if err != nil || phone.User != "5511999999999" || display != "+5511999999999" {
		t.Fatalf("phone parsing failed: %v %s %q", err, phone, display)
	}
	group, display, err := parseMessageRecipient("120363000000000000@g.us")
	if err != nil || group.Server != "g.us" || display != "120363000000000000@g.us" {
		t.Fatalf("group parsing failed: %v %s %q", err, group, display)
	}
}
