package engine

import (
	"fmt"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Event struct {
	ID         string         `json:"id"`
	Event      string         `json:"event"`
	InstanceID string         `json:"instanceId"`
	Timestamp  string         `json:"timestamp"`
	Data       map[string]any `json:"data"`
}

type EventHandler func(Event)

func (m *Manager) SetEventHandler(handler EventHandler) {
	m.eventMu.Lock()
	m.eventHandler = handler
	m.eventMu.Unlock()
}

func (m *Manager) emit(event Event) {
	m.eventMu.RLock()
	handler := m.eventHandler
	m.eventMu.RUnlock()
	if handler != nil {
		go handler(event)
	}
}

func newEvent(eventName, instanceID string, timestamp time.Time, data map[string]any) Event {
	identifier, err := security.RandomToken(18)
	if err != nil {
		identifier = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return Event{
		ID: "evt_" + identifier, Event: eventName, InstanceID: instanceID,
		Timestamp: timestamp.UTC().Format(time.RFC3339), Data: data,
	}
}

func incomingMessageEvent(instanceID string, message *events.Message) Event {
	eventName := "message.received"
	if message.Info.IsFromMe {
		eventName = "message.sent"
	}
	if message.IsEdit {
		eventName = "message.updated"
	}
	if message.Message.GetReactionMessage() != nil {
		eventName = "message.reaction"
	}
	protocol := message.Message.GetProtocolMessage()
	if protocol != nil {
		if protocol.GetType() == waE2E.ProtocolMessage_REVOKE {
			eventName = "message.deleted"
		}
		if protocol.GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT {
			eventName = "message.updated"
		}
	}
	if message.Info.Chat == types.StatusBroadcastJID {
		eventName = "status.received"
		if message.Info.IsFromMe {
			eventName = "status.sent"
		}
	}
	messageType := message.Info.MediaType
	if messageType == "" {
		messageType = message.Info.Type
	}
	data := map[string]any{
		"id":       string(message.Info.ID),
		"from":     message.Info.Sender.ToNonAD().String(),
		"chat":     message.Info.Chat.ToNonAD().String(),
		"fromMe":   message.Info.IsFromMe,
		"isGroup":  message.Info.IsGroup,
		"pushName": message.Info.PushName,
		"type":     messageType,
		"text":     messageText(message),
	}
	if reaction := message.Message.GetReactionMessage(); reaction != nil {
		data["reaction"] = reaction.GetText()
		data["targetId"] = reaction.GetKey().GetID()
	}
	if protocol != nil {
		data["targetId"] = protocol.GetKey().GetID()
		if edited := protocol.GetEditedMessage(); edited != nil {
			data["text"] = messageText(&events.Message{Message: edited})
		}
	}
	return newEvent(eventName, instanceID, message.Info.Timestamp, data)
}

func messageText(event *events.Message) string {
	message := event.Message
	if message == nil {
		return ""
	}
	switch {
	case message.GetConversation() != "":
		return message.GetConversation()
	case message.GetExtendedTextMessage() != nil:
		return message.GetExtendedTextMessage().GetText()
	case message.GetImageMessage() != nil:
		return message.GetImageMessage().GetCaption()
	case message.GetVideoMessage() != nil:
		return message.GetVideoMessage().GetCaption()
	case message.GetDocumentMessage() != nil:
		return message.GetDocumentMessage().GetCaption()
	default:
		return ""
	}
}

func receiptEvent(id string, value *events.Receipt) Event {
	return newEvent("message.receipt", id, value.Timestamp, map[string]any{
		"ids": value.MessageIDs, "chat": value.Chat.ToNonAD().String(),
		"from": value.Sender.ToNonAD().String(), "type": string(value.Type),
	})
}
func presenceEvent(id string, value *events.Presence) Event {
	data := map[string]any{"from": value.From.ToNonAD().String(), "unavailable": value.Unavailable}
	if !value.LastSeen.IsZero() {
		data["lastSeen"] = value.LastSeen.UTC().Format(time.RFC3339)
	}
	return newEvent("presence.updated", id, time.Now().UTC(), data)
}
func chatPresenceEvent(id string, value *events.ChatPresence) Event {
	return newEvent("presence.chat", id, time.Now().UTC(), map[string]any{
		"chat": value.Chat.ToNonAD().String(), "from": value.Sender.ToNonAD().String(), "state": string(value.State), "media": string(value.Media),
	})
}
func groupEvent(id string, value *events.GroupInfo) Event {
	data := map[string]any{"id": value.JID.String(), "joined": value.Join, "left": value.Leave, "promoted": value.Promote, "demoted": value.Demote}
	if value.Sender != nil {
		data["from"] = value.Sender.ToNonAD().String()
	}
	if value.Name != nil {
		data["name"] = value.Name.Name
	}
	if value.Topic != nil {
		data["description"] = value.Topic.Topic
	}
	return newEvent("group.updated", id, value.Timestamp, data)
}

func callEvent(id, eventName string, meta types.BasicCallMeta, remotePlatform, remoteVersion, reason string) Event {
	data := map[string]any{
		"id": meta.CallID, "from": meta.From.ToNonAD().String(),
		"creator": meta.CallCreator.ToNonAD().String(),
	}
	if !meta.GroupJID.IsEmpty() {
		data["group"] = meta.GroupJID.String()
	}
	if remotePlatform != "" {
		data["remotePlatform"] = remotePlatform
	}
	if remoteVersion != "" {
		data["remoteVersion"] = remoteVersion
	}
	if reason != "" {
		data["reason"] = reason
	}
	return newEvent(eventName, id, meta.Timestamp, data)
}
