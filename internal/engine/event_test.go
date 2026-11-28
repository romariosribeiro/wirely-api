package engine

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestIncomingMessageEvent(t *testing.T) {
	timestamp := time.Date(2026, time.September, 16, 20, 30, 0, 0, time.UTC)
	event := incomingMessageEvent("instance-1", &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    types.NewJID("5511999999999", types.DefaultUserServer),
				Sender:  types.NewJID("5511888888888", types.DefaultUserServer),
				IsGroup: false, IsFromMe: false,
			},
			ID: "message-1", PushName: "Customer", Timestamp: timestamp, Type: "text",
		},
		Message: &waE2E.Message{Conversation: proto.String("Hello Wirely")},
	})

	if event.Event != "message.received" || event.InstanceID != "instance-1" {
		t.Fatalf("unexpected event envelope: %#v", event)
	}
	if event.Timestamp != timestamp.Format(time.RFC3339) {
		t.Fatalf("unexpected timestamp: %s", event.Timestamp)
	}
	if event.Data["id"] != "message-1" || event.Data["text"] != "Hello Wirely" {
		t.Fatalf("unexpected message data: %#v", event.Data)
	}
	if event.Data["from"] != "5511888888888@s.whatsapp.net" || event.Data["chat"] != "5511999999999@s.whatsapp.net" {
		t.Fatalf("unexpected JIDs: %#v", event.Data)
	}
	if event.Data["phone"] != "5511888888888" {
		t.Fatalf("missing sender phone: %#v", event.Data)
	}
}

func TestMessageTextUsesMediaCaption(t *testing.T) {
	message := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{Caption: proto.String("Photo caption")},
	}
	if text := messageText(message); text != "Photo caption" {
		t.Fatalf("unexpected media caption: %q", text)
	}
}

func TestEditedMessageTextUnwrapsGroupMention(t *testing.T) {
	message := &events.Message{Message: &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			EditedMessage: &waE2E.Message{GroupMentionedMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{Conversation: proto.String("Updated message")},
			}},
		},
	}}
	event := incomingMessageEvent("instance-1", message)
	if event.Event != "message.updated" || event.Data["text"] != "Updated message" {
		t.Fatalf("unexpected edited message: %#v", event)
	}
}
