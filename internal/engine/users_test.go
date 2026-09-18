package engine

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestMapBlockedUsersNormalizesAndSorts(t *testing.T) {
	users := mapBlockedUsers([]types.JID{
		types.NewJID("5511999999999", types.DefaultUserServer),
		types.NewJID("5511888888888", types.DefaultUserServer),
	})
	if len(users) != 2 || users[0].Phone != "5511888888888" || users[1].JID != "5511999999999@s.whatsapp.net" {
		t.Fatalf("unexpected users: %#v", users)
	}
}
