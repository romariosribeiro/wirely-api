package engine

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type senderLIDStore struct {
	whatsmeowStore.NoopStore
	phone types.JID
	err   error
	calls []types.JID
}

func (s *senderLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	s.calls = append(s.calls, lid)
	return s.phone, s.err
}

func TestIncomingMessageSenderPhone(t *testing.T) {
	phone := types.NewJID("5511999999999", types.DefaultUserServer)
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	group := types.NewJID("120363000000000000", types.GroupServer)
	for _, test := range []struct {
		name                            string
		sender, alternate, chat, mapped types.JID
		recipient                       types.JID
		wantPhone                       string
		wantLookup, fromMe, noStore     bool
		lookupError                     error
	}{
		{name: "phone sender", sender: phone, chat: phone, wantPhone: phone.User},
		{name: "phone sender with device", sender: types.JID{User: phone.User, Server: phone.Server, Device: 7}, chat: phone, wantPhone: phone.User},
		{name: "sender wins over alternate", sender: phone, alternate: types.NewJID("5511888888888", types.DefaultUserServer), chat: phone, wantPhone: phone.User},
		{name: "LID with alternate phone", sender: lid, alternate: phone, chat: lid, wantPhone: phone.User},
		{name: "group author alternate phone", sender: lid, alternate: phone, chat: group, wantPhone: phone.User},
		{name: "cached LID phone", sender: lid, chat: lid, mapped: phone, wantPhone: phone.User, wantLookup: true},
		{name: "group cached author", sender: lid, chat: group, mapped: phone, wantPhone: phone.User, wantLookup: true},
		{name: "cached LID with device", sender: types.JID{User: lid.User, Server: lid.Server, Device: 4}, chat: lid, mapped: phone, wantPhone: phone.User, wantLookup: true},
		{name: "unknown LID", sender: lid, chat: lid, wantLookup: true},
		{name: "lookup failure keeps event", sender: lid, chat: group, wantLookup: true, lookupError: errors.New("lookup failed")},
		{name: "lookup timeout keeps event", sender: lid, chat: lid, wantLookup: true, lookupError: context.DeadlineExceeded},
		{name: "mapping must contain phone JID", sender: lid, chat: lid, mapped: lid, wantLookup: true},
		{name: "recipient is not sender", sender: lid, chat: phone, recipient: phone, fromMe: true, wantLookup: true},
		{name: "no sender uses neither chat nor recipient", chat: phone, recipient: phone},
		{name: "group ID is not phone", sender: group, chat: group},
		{name: "malformed phone is not exposed", sender: types.NewJID("5511999999999abc", types.DefaultUserServer), chat: phone},
		{name: "invalid mapped phone", sender: lid, chat: lid, mapped: types.NewJID("00000000000", types.DefaultUserServer), wantLookup: true},
		{name: "no mapping store", sender: lid, chat: lid, noStore: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			lookup := &senderLIDStore{phone: test.mapped, err: test.lookupError}
			current := &session{id: "instance-1"}
			if !test.noStore {
				current.client = &whatsmeow.Client{Store: &whatsmeowStore.Device{LIDs: lookup}}
			}
			message := &events.Message{
				Info: types.MessageInfo{MessageSource: types.MessageSource{
					Sender: test.sender, SenderAlt: test.alternate, RecipientAlt: test.recipient,
					Chat: test.chat, IsGroup: test.chat.Server == types.GroupServer, IsFromMe: test.fromMe,
				}, ID: "MSG-1", Type: "text"},
				Message: &waE2E.Message{Conversation: proto.String("Hello")},
			}
			event := (&Manager{}).prepareIncomingMessage(current, message)
			phone, present := event.Data["phone"]
			if (test.wantPhone != "" && phone != test.wantPhone) || (test.wantPhone == "" && present) {
				t.Fatalf("incorrect phone: %#v", event.Data)
			}
			if event.Data["from"] != test.sender.ToNonAD().String() || event.Data["chat"] != test.chat.ToNonAD().String() ||
				event.Data["text"] != "Hello" || event.InstanceID != current.id {
				t.Fatalf("existing event fields changed: %#v", event)
			}
			if (len(lookup.calls) == 1) != test.wantLookup {
				t.Fatalf("unexpected lookup count: %d", len(lookup.calls))
			}
			if test.wantLookup && lookup.calls[0] != test.sender.ToNonAD() {
				t.Fatalf("looked up another sender: %v", lookup.calls)
			}
		})
	}
}

func TestIncomingMediaIncludesSenderPhone(t *testing.T) {
	sender := types.NewJID("123456789012345", types.HiddenUserServer)
	message := &events.Message{
		Info: types.MessageInfo{MessageSource: types.MessageSource{
			Sender: sender, SenderAlt: types.NewJID("5511999999999", types.DefaultUserServer), Chat: sender,
		}, ID: "IMAGE-1"},
		Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Mimetype: proto.String("image/jpeg"), FileLength: proto.Uint64(MaxReceivedMediaBytes + 1),
		}},
	}
	event := (&Manager{}).prepareIncomingMessage(&session{id: "instance-1"}, message)
	if event.Data["phone"] != "5511999999999" || event.Data["from"] != sender.String() || event.Data["type"] != "image" || event.Data["media"] == nil {
		t.Fatalf("media event lost sender identity: %#v", event.Data)
	}
}
