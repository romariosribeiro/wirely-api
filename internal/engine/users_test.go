package engine

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestMapBlockedUsersNormalizesAndSorts(t *testing.T) {
	users := mapBlockedUsers(&types.Blocklist{Items: []types.BlocklistItem{
		{LID: types.JID{User: "5511999999999", Server: types.DefaultUserServer, Device: 1}, Active: true},
		{LID: types.NewJID("5511888888888", types.DefaultUserServer), Active: true},
	}})
	if len(users) != 2 || users[0].Phone != "5511888888888" || users[1].JID != "5511999999999@s.whatsapp.net" {
		t.Fatalf("unexpected users: %#v", users)
	}
}

func TestMapBlockedUsersHandlesActiveLIDAndPhoneEntries(t *testing.T) {
	users := mapBlockedUsers(&types.Blocklist{Items: []types.BlocklistItem{
		{LID: types.NewJID("100", types.HiddenUserServer), PN: types.NewJID("5511999999999", types.DefaultUserServer), Active: true},
		{LID: types.NewJID("200", types.HiddenUserServer), Active: true},
		{PN: types.NewJID("5511888888888", types.DefaultUserServer), Active: true},
		{LID: types.NewJID("300", types.HiddenUserServer), PN: types.NewJID("5511777777777", types.DefaultUserServer), Active: false},
		{Active: true},
	}})
	if len(users) != 3 || users[0].JID != "100@lid" || users[0].Phone != "5511999999999" ||
		users[1].JID != "200@lid" || users[1].Phone != "" || users[2].Phone != "5511888888888" {
		t.Fatalf("unexpected active blocked users: %#v", users)
	}
}

func TestMapBlockedUsersEmptyList(t *testing.T) {
	for _, list := range []*types.Blocklist{nil, {}, {Items: []types.BlocklistItem{
		{LID: types.NewJID("100", types.HiddenUserServer), Active: false},
	}}} {
		users := mapBlockedUsers(list)
		if users == nil || len(users) != 0 {
			t.Fatalf("expected an empty array, got %#v", users)
		}
	}
}
