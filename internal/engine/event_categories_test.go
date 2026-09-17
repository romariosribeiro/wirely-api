package engine

import (
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestMessageWebhookCategories(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message *events.Message
		want    string
	}{
		{"incoming", &events.Message{Message: &waE2E.Message{Conversation: proto.String("hello")}}, "message.received"},
		{"outgoing", &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{IsFromMe: true}}}, "message.sent"},
		{"edit", &events.Message{IsEdit: true}, "message.updated"},
		{"reaction", &events.Message{Message: &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍"), Key: &waCommon.MessageKey{ID: proto.String("target")}}}}, "message.reaction"},
		{"delete", &events.Message{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}}, "message.deleted"},
		{"status", &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID}}}, "status.received"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := incomingMessageEvent("instance", tc.message)
			if result.Event != tc.want {
				t.Fatalf("got %s, want %s", result.Event, tc.want)
			}
			if tc.name == "reaction" && result.Data["targetId"] != "target" {
				t.Fatal("reaction target missing")
			}
		})
	}
}
