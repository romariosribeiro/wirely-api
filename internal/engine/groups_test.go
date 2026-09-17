package engine

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestValidateGroupName(t *testing.T) {
	name, err := validateGroupName("  Equipe Wirely  ")
	if err != nil || name != "Equipe Wirely" {
		t.Fatalf("unexpected name validation: %q %v", name, err)
	}
	if _, err := validateGroupName(""); err == nil {
		t.Fatal("empty group name accepted")
	}
	if _, err := validateGroupName(strings.Repeat("x", 26)); err == nil {
		t.Fatal("long group name accepted")
	}
}

func TestParseGroupParticipants(t *testing.T) {
	participants, err := parseGroupParticipants([]string{"+5511999999999", "5511888888888@s.whatsapp.net"})
	if err != nil || len(participants) != 2 || participants[0].Server != types.DefaultUserServer {
		t.Fatalf("unexpected participants: %#v %v", participants, err)
	}
	for _, invalid := range [][]string{
		{},
		{"+5511999999999", "5511999999999@s.whatsapp.net"},
		{"120363000000000000@g.us"},
	} {
		if _, err := parseGroupParticipants(invalid); err == nil {
			t.Fatalf("invalid participants accepted: %#v", invalid)
		}
	}
}

func TestMapGroup(t *testing.T) {
	group := mapGroup(&types.GroupInfo{
		JID:       types.NewJID("120363000000000000", types.GroupServer),
		GroupName: types.GroupName{Name: "Wirely"},
		Participants: []types.GroupParticipant{{
			JID: types.NewJID("5511999999999", types.DefaultUserServer), IsAdmin: true,
		}},
	})
	if group.JID != "120363000000000000@g.us" || group.Name != "Wirely" ||
		group.ParticipantCount != 1 || !group.Participants[0].IsAdmin {
		t.Fatalf("unexpected mapped group: %#v", group)
	}
}
