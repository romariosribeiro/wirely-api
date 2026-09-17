package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type LocationPayload struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
}

type ContactPayload struct {
	FullName     string `json:"fullName"`
	Organization string `json:"organization,omitempty"`
	Phone        string `json:"phone"`
}

type PollPayload struct {
	Question  string   `json:"question"`
	Options   []string `json:"options"`
	MaxAnswer int      `json:"maxAnswer"`
}

type ReactionPayload struct {
	MessageID   string `json:"messageId"`
	Reaction    string `json:"reaction"`
	FromMe      bool   `json:"fromMe,omitempty"`
	Participant string `json:"participant,omitempty"`
}

func (m *Manager) SendLocation(ctx context.Context, id, recipient string, payload LocationPayload) (SentMessage, error) {
	if err := ValidateLocation(&payload); err != nil {
		return SentMessage{}, err
	}
	message := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude: proto.Float64(payload.Latitude), DegreesLongitude: proto.Float64(payload.Longitude),
		Name: proto.String(payload.Name), Address: proto.String(payload.Address),
	}}
	return m.sendStructured(ctx, id, recipient, "location", message, map[string]any{
		"latitude": payload.Latitude, "longitude": payload.Longitude, "name": payload.Name, "address": payload.Address,
	})
}

func (m *Manager) SendContact(ctx context.Context, id, recipient string, payload ContactPayload) (SentMessage, error) {
	if err := ValidateContact(&payload); err != nil {
		return SentMessage{}, err
	}
	message := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: proto.String(payload.FullName), Vcard: proto.String(contactVCard(payload)),
	}}
	return m.sendStructured(ctx, id, recipient, "contact", message, map[string]any{
		"contactName": payload.FullName, "contactPhone": "+" + payload.Phone,
	})
}

func (m *Manager) SendPoll(ctx context.Context, id, recipient string, payload PollPayload) (SentMessage, error) {
	if err := ValidatePoll(&payload); err != nil {
		return SentMessage{}, err
	}
	current, err := m.connectedSession(id)
	if err != nil {
		return SentMessage{}, err
	}
	jid, display, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return SentMessage{}, err
	}
	message := current.client.BuildPollCreation(payload.Question, payload.Options, payload.MaxAnswer)
	return m.sendStructuredWithSession(ctx, id, current, jid, display, "poll", message, map[string]any{
		"question": payload.Question, "options": payload.Options, "maxAnswer": payload.MaxAnswer,
	})
}

func (m *Manager) SendReaction(ctx context.Context, id, recipient string, payload ReactionPayload) (SentMessage, error) {
	if err := ValidateReaction(&payload); err != nil {
		return SentMessage{}, err
	}
	current, err := m.connectedSession(id)
	if err != nil {
		return SentMessage{}, err
	}
	chat, display, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return SentMessage{}, err
	}
	sender := types.EmptyJID
	if !payload.FromMe {
		sender = chat
		if chat.Server == types.GroupServer {
			if payload.Participant == "" {
				return SentMessage{}, errors.New("participant is required when reacting to a received group message")
			}
			sender, _, err = parseMessageRecipient(payload.Participant)
			if err != nil {
				return SentMessage{}, fmt.Errorf("invalid participant: %w", err)
			}
		}
	}
	message := current.client.BuildReaction(chat, sender, types.MessageID(payload.MessageID), payload.Reaction)
	return m.sendStructuredWithSession(ctx, id, current, chat, display, "reaction", message, map[string]any{
		"targetMessageId": payload.MessageID, "reaction": payload.Reaction,
	})
}

func (m *Manager) sendStructured(ctx context.Context, id, recipient, kind string, message *waE2E.Message, fields map[string]any) (SentMessage, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return SentMessage{}, err
	}
	jid, display, err := resolveMessageRecipient(ctx, current.client, recipient)
	if err != nil {
		return SentMessage{}, err
	}
	return m.sendStructuredWithSession(ctx, id, current, jid, display, kind, message, fields)
}

func (m *Manager) sendStructuredWithSession(ctx context.Context, id string, current *session, jid types.JID, display, kind string, message *waE2E.Message, fields map[string]any) (SentMessage, error) {
	response, err := current.client.SendMessage(ctx, jid, message)
	if err != nil {
		return SentMessage{}, fmt.Errorf("send WhatsApp %s: %w", kind, err)
	}
	timestamp := response.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	eventFields := map[string]any{"id": string(response.ID), "chat": jid.String(), "fromMe": true, "isGroup": jid.Server == types.GroupServer, "type": kind}
	for key, value := range fields {
		eventFields[key] = value
	}
	m.emit(newEvent("message.sent", id, timestamp, eventFields))
	return SentMessage{ID: string(response.ID), Recipient: display, Timestamp: timestamp.Format(time.RFC3339), Type: kind}, nil
}

func (m *Manager) connectedSession(id string) (*session, error) {
	current, err := m.get(id)
	if err != nil {
		return nil, err
	}
	if !current.client.IsConnected() || !current.client.IsLoggedIn() {
		return nil, ErrNotConnected
	}
	return current, nil
}

func parseMessageRecipient(value string) (types.JID, string, error) {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "@") {
		jid, err := types.ParseJID(value)
		if err != nil || jid.User == "" || (jid.Server != types.DefaultUserServer && jid.Server != types.HiddenUserServer && jid.Server != types.GroupServer) {
			return types.EmptyJID, "", errors.New("recipient must be a phone number with country code or a valid WhatsApp chat JID")
		}
		jid = jid.ToNonAD()
		return jid, jid.String(), nil
	}
	phone, err := NormalizeRecipient(value)
	if err != nil {
		return types.EmptyJID, "", err
	}
	return types.NewJID(phone, types.DefaultUserServer), "+" + phone, nil
}

func ValidateLocation(payload *LocationPayload) error {
	if math.IsNaN(payload.Latitude) || math.IsInf(payload.Latitude, 0) || payload.Latitude < -90 || payload.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if math.IsNaN(payload.Longitude) || math.IsInf(payload.Longitude, 0) || payload.Longitude < -180 || payload.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Address = strings.TrimSpace(payload.Address)
	if utf8.RuneCountInString(payload.Name) > 200 {
		return errors.New("name must have at most 200 characters")
	}
	if utf8.RuneCountInString(payload.Address) > 500 {
		return errors.New("address must have at most 500 characters")
	}
	return nil
}

func ValidateContact(payload *ContactPayload) error {
	payload.FullName = strings.TrimSpace(payload.FullName)
	payload.Organization = strings.TrimSpace(payload.Organization)
	if payload.FullName == "" {
		return errors.New("fullName is required")
	}
	if utf8.RuneCountInString(payload.FullName) > 200 || utf8.RuneCountInString(payload.Organization) > 200 {
		return errors.New("fullName and organization must have at most 200 characters")
	}
	phone, err := NormalizeRecipient(payload.Phone)
	if err != nil {
		return fmt.Errorf("invalid contact phone: %w", err)
	}
	payload.Phone = phone
	return nil
}

func ValidatePoll(payload *PollPayload) error {
	payload.Question = strings.TrimSpace(payload.Question)
	if payload.Question == "" {
		return errors.New("question is required")
	}
	if utf8.RuneCountInString(payload.Question) > 255 {
		return errors.New("question must have at most 255 characters")
	}
	if len(payload.Options) < 2 || len(payload.Options) > 12 {
		return errors.New("options must contain between 2 and 12 items")
	}
	seen := make(map[string]struct{}, len(payload.Options))
	for index := range payload.Options {
		option := strings.TrimSpace(payload.Options[index])
		if option == "" {
			return errors.New("poll options cannot be empty")
		}
		if utf8.RuneCountInString(option) > 100 {
			return errors.New("each poll option must have at most 100 characters")
		}
		key := strings.ToLower(option)
		if _, exists := seen[key]; exists {
			return errors.New("poll options must be unique")
		}
		seen[key] = struct{}{}
		payload.Options[index] = option
	}
	if payload.MaxAnswer == 0 {
		payload.MaxAnswer = 1
	}
	if payload.MaxAnswer < 1 || payload.MaxAnswer > len(payload.Options) {
		return errors.New("maxAnswer must be between 1 and the number of options")
	}
	return nil
}

func ValidateReaction(payload *ReactionPayload) error {
	payload.MessageID = strings.TrimSpace(payload.MessageID)
	payload.Reaction = strings.TrimSpace(payload.Reaction)
	payload.Participant = strings.TrimSpace(payload.Participant)
	if payload.MessageID == "" {
		return errors.New("messageId is required")
	}
	if len(payload.MessageID) > 200 {
		return errors.New("messageId must have at most 200 characters")
	}
	if utf8.RuneCountInString(payload.Reaction) > 8 {
		return errors.New("reaction must have at most 8 characters")
	}
	return nil
}

func contactVCard(payload ContactPayload) string {
	escape := func(value string) string {
		value = strings.ReplaceAll(value, "\\", "\\\\")
		value = strings.ReplaceAll(value, ";", "\\;")
		value = strings.ReplaceAll(value, ",", "\\,")
		value = strings.ReplaceAll(value, "\r", "")
		return strings.ReplaceAll(value, "\n", "\\n")
	}
	lines := []string{"BEGIN:VCARD", "VERSION:3.0", "FN:" + escape(payload.FullName)}
	if payload.Organization != "" {
		lines = append(lines, "ORG:"+escape(payload.Organization))
	}
	lines = append(lines, "TEL;TYPE=CELL;TYPE=VOICE;WAID="+payload.Phone+":"+payload.Phone, "END:VCARD")
	return strings.Join(lines, "\r\n")
}
