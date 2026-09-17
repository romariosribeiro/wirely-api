package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type Contact struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	Phone        string `json:"phone,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	FullName     string `json:"fullName,omitempty"`
	PushName     string `json:"pushName,omitempty"`
	BusinessName string `json:"businessName,omitempty"`
}

func (m *Manager) Contacts(ctx context.Context, id string) ([]Contact, error) {
	current, err := m.get(id)
	if err != nil {
		return nil, err
	}
	if current.client.Store.Contacts == nil {
		return nil, errors.New("contact store is unavailable")
	}
	stored, err := current.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list WhatsApp contacts: %w", err)
	}
	contacts := make([]Contact, 0, len(stored))
	for jid, info := range stored {
		jid = jid.ToNonAD()
		if jid.User == "" || (jid.Server != types.DefaultUserServer && jid.Server != types.HostedServer) {
			continue
		}
		phone := ""
		if jid.Server == types.DefaultUserServer {
			phone = "+" + jid.User
		}
		name := firstNotEmpty(info.FullName, info.BusinessName, info.PushName, info.FirstName, phone, jid.String())
		contacts = append(contacts, Contact{
			JID: jid.String(), Name: name, Phone: phone, FirstName: info.FirstName,
			FullName: info.FullName, PushName: info.PushName, BusinessName: info.BusinessName,
		})
	}
	sort.Slice(contacts, func(i, j int) bool {
		return strings.ToLower(contacts[i].Name) < strings.ToLower(contacts[j].Name)
	})
	return contacts, nil
}

func firstNotEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (m *Manager) SendChatText(ctx context.Context, id, chat, message string) (SentMessage, error) {
	jid, err := parseChatJID(chat)
	if err != nil {
		return SentMessage{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return SentMessage{}, errors.New("message is required")
	}
	if len([]rune(message)) > 4096 {
		return SentMessage{}, errors.New("message must have at most 4096 characters")
	}
	current, err := m.get(id)
	if err != nil {
		return SentMessage{}, err
	}
	if !current.client.IsConnected() || !current.client.IsLoggedIn() {
		return SentMessage{}, ErrNotConnected
	}
	response, err := current.client.SendMessage(ctx, jid, &waE2E.Message{Conversation: proto.String(message)})
	if err != nil {
		return SentMessage{}, fmt.Errorf("send WhatsApp message: %w", err)
	}
	timestamp := response.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	m.emit(newEvent("message.sent", id, timestamp, map[string]any{
		"id": string(response.ID), "chat": jid.String(), "fromMe": true,
		"isGroup": jid.Server == types.GroupServer, "type": "text", "text": message,
	}))
	return SentMessage{ID: string(response.ID), Recipient: jid.String(), Timestamp: timestamp.Format(time.RFC3339), Type: "text"}, nil
}

func parseChatJID(value string) (types.JID, error) {
	jid, err := types.ParseJID(strings.TrimSpace(value))
	if err != nil {
		return types.EmptyJID, errors.New("invalid chat identifier")
	}
	jid = jid.ToNonAD()
	if jid.User == "" {
		return types.EmptyJID, errors.New("invalid chat identifier")
	}
	switch jid.Server {
	case types.DefaultUserServer, types.GroupServer, types.HiddenUserServer, types.HostedServer, types.HostedLIDServer:
		return jid, nil
	default:
		return types.EmptyJID, errors.New("unsupported chat identifier")
	}
}
