package engine

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

var firstURL = regexp.MustCompile(`https?://[^\s]+`)

type ReplyOptions struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant,omitempty"`
	Text        string `json:"text,omitempty"`
}

type MessageOptions struct {
	ReplyTo     *ReplyOptions `json:"replyTo,omitempty"`
	Mentions    []string      `json:"mentions,omitempty"`
	Forwarded   bool          `json:"forwarded,omitempty"`
	LinkPreview bool          `json:"linkPreview,omitempty"`
}

func buildMessageContext(chat types.JID, options MessageOptions) (*waE2E.ContextInfo, error) {
	contextInfo := &waE2E.ContextInfo{}
	if options.ReplyTo != nil {
		messageID := strings.TrimSpace(options.ReplyTo.MessageID)
		if messageID == "" || len(messageID) > 200 {
			return nil, errors.New("replyTo.messageId is required and must have at most 200 characters")
		}
		participant := chat
		if chat.Server == types.GroupServer {
			if strings.TrimSpace(options.ReplyTo.Participant) == "" {
				return nil, errors.New("replyTo.participant is required for group messages")
			}
			parsed, _, err := parseMessageRecipient(options.ReplyTo.Participant)
			if err != nil {
				return nil, fmt.Errorf("invalid replyTo.participant: %w", err)
			}
			participant = parsed
		} else if strings.TrimSpace(options.ReplyTo.Participant) != "" {
			return nil, errors.New("replyTo.participant is only supported for groups")
		}
		quotedText := strings.TrimSpace(options.ReplyTo.Text)
		contextInfo.StanzaID = proto.String(messageID)
		contextInfo.Participant = proto.String(participant.String())
		contextInfo.RemoteJID = proto.String(chat.String())
		contextInfo.QuotedMessage = &waE2E.Message{Conversation: proto.String(quotedText)}
	}
	if len(options.Mentions) > 100 {
		return nil, errors.New("mentions must contain at most 100 contacts")
	}
	seen := make(map[string]struct{}, len(options.Mentions))
	for _, value := range options.Mentions {
		jid, _, err := parseMessageRecipient(value)
		if err != nil || jid.Server == types.GroupServer {
			return nil, fmt.Errorf("invalid mention %q", value)
		}
		jid = jid.ToNonAD()
		if _, exists := seen[jid.String()]; exists {
			continue
		}
		seen[jid.String()] = struct{}{}
		contextInfo.MentionedJID = append(contextInfo.MentionedJID, jid.String())
	}
	if options.Forwarded {
		contextInfo.IsForwarded = proto.Bool(true)
		contextInfo.ForwardingScore = proto.Uint32(1)
	}
	if contextInfo.StanzaID == nil && len(contextInfo.MentionedJID) == 0 && !contextInfo.GetIsForwarded() {
		return nil, nil
	}
	return contextInfo, nil
}

func textMessage(text string, chat types.JID, options MessageOptions) (*waE2E.Message, error) {
	contextInfo, err := buildMessageContext(chat, options)
	if err != nil {
		return nil, err
	}
	if contextInfo == nil && !options.LinkPreview {
		return &waE2E.Message{Conversation: proto.String(text)}, nil
	}
	extended := &waE2E.ExtendedTextMessage{Text: proto.String(text), ContextInfo: contextInfo}
	if options.LinkPreview {
		matched := firstURL.FindString(text)
		if matched == "" {
			return nil, errors.New("linkPreview requires an HTTP or HTTPS URL in message")
		}
		extended.MatchedText = proto.String(matched)
	}
	return &waE2E.Message{ExtendedTextMessage: extended}, nil
}

func applyMessageOptions(message *waE2E.Message, chat types.JID, options MessageOptions) error {
	contextInfo, err := buildMessageContext(chat, options)
	if err != nil {
		return err
	}
	if options.LinkPreview {
		return errors.New("linkPreview is only supported for text messages")
	}
	if contextInfo == nil {
		return nil
	}
	switch {
	case message.ImageMessage != nil:
		message.ImageMessage.ContextInfo = contextInfo
	case message.VideoMessage != nil:
		message.VideoMessage.ContextInfo = contextInfo
	case message.AudioMessage != nil:
		message.AudioMessage.ContextInfo = contextInfo
	case message.DocumentMessage != nil:
		message.DocumentMessage.ContextInfo = contextInfo
	case message.StickerMessage != nil:
		message.StickerMessage.ContextInfo = contextInfo
	case message.LocationMessage != nil:
		message.LocationMessage.ContextInfo = contextInfo
	case message.LiveLocationMessage != nil:
		message.LiveLocationMessage.ContextInfo = contextInfo
	case message.ContactMessage != nil:
		message.ContactMessage.ContextInfo = contextInfo
	default:
		return errors.New("message type does not support reply, mentions, or forwarded context")
	}
	return nil
}
