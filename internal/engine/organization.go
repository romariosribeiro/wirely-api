package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Newsletter struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	InviteCode   string `json:"inviteCode,omitempty"`
	Subscribers  int    `json:"subscribers"`
	State        string `json:"state"`
	Role         string `json:"role,omitempty"`
	Mute         string `json:"mute,omitempty"`
	Verification string `json:"verification,omitempty"`
}

type NewsletterMessage struct {
	ServerID  int64          `json:"serverId"`
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	Text      string         `json:"text,omitempty"`
	Views     int            `json:"views"`
	Reactions map[string]int `json:"reactions,omitempty"`
}

type LabelResult struct {
	LabelID   string `json:"labelId"`
	Target    string `json:"target,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Labeled   *bool  `json:"labeled,omitempty"`
	Name      string `json:"name,omitempty"`
	Color     int32  `json:"color,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

type CommunityGroupsResult struct {
	Success []string          `json:"success"`
	Failed  map[string]string `json:"failed,omitempty"`
}

func (m *Manager) CreateNewsletter(ctx context.Context, id, name, description string) (Newsletter, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Newsletter{}, err
	}
	name, description, err = validateNewsletter(name, description)
	if err != nil {
		return Newsletter{}, err
	}
	metadata, err := current.client.CreateNewsletter(ctx, whatsmeow.CreateNewsletterParams{Name: name, Description: description})
	if err != nil {
		return Newsletter{}, fmt.Errorf("create WhatsApp newsletter: %w", err)
	}
	return mapNewsletter(metadata), nil
}

func (m *Manager) GetNewsletter(ctx context.Context, id, newsletterJID string) (Newsletter, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Newsletter{}, err
	}
	jid, err := parseNewsletterJID(newsletterJID)
	if err != nil {
		return Newsletter{}, err
	}
	metadata, err := current.client.GetNewsletterInfo(ctx, jid)
	if err != nil {
		return Newsletter{}, fmt.Errorf("get WhatsApp newsletter: %w", err)
	}
	return mapNewsletter(metadata), nil
}

func (m *Manager) GetNewsletterByInvite(ctx context.Context, id, key string) (Newsletter, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Newsletter{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 512 {
		return Newsletter{}, errors.New("a valid newsletter invite code or link is required")
	}
	metadata, err := current.client.GetNewsletterInfoWithInvite(ctx, key)
	if err != nil {
		return Newsletter{}, fmt.Errorf("get WhatsApp newsletter invite: %w", err)
	}
	return mapNewsletter(metadata), nil
}

func (m *Manager) ListNewsletters(ctx context.Context, id string) ([]Newsletter, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	items, err := current.client.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, fmt.Errorf("list WhatsApp newsletters: %w", err)
	}
	result := make([]Newsletter, 0, len(items))
	for _, item := range items {
		result = append(result, mapNewsletter(item))
	}
	return result, nil
}

func (m *Manager) GetNewsletterMessages(ctx context.Context, id, newsletterJID string, count int, before int64) ([]NewsletterMessage, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	jid, err := parseNewsletterJID(newsletterJID)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		count = 20
	}
	if count < 1 || count > 100 || before < 0 {
		return nil, errors.New("count must be between 1 and 100 and before cannot be negative")
	}
	items, err := current.client.GetNewsletterMessages(ctx, jid, &whatsmeow.GetNewsletterMessagesParams{Count: count, Before: types.MessageServerID(before)})
	if err != nil {
		return nil, fmt.Errorf("get WhatsApp newsletter messages: %w", err)
	}
	result := make([]NewsletterMessage, 0, len(items))
	for _, item := range items {
		result = append(result, NewsletterMessage{ServerID: int64(item.MessageServerID), ID: string(item.MessageID), Type: item.Type,
			Timestamp: item.Timestamp.UTC().Format("2006-01-02T15:04:05Z07:00"), Text: messageText(&events.Message{Message: item.Message}), Views: item.ViewsCount, Reactions: item.ReactionCounts})
	}
	return result, nil
}

func (m *Manager) SubscribeNewsletter(ctx context.Context, id, newsletterJID string) error {
	current, err := m.connectedSession(id)
	if err != nil {
		return err
	}
	jid, err := parseNewsletterJID(newsletterJID)
	if err != nil {
		return err
	}
	if err := current.client.FollowNewsletter(ctx, jid); err != nil {
		return fmt.Errorf("subscribe to WhatsApp newsletter: %w", err)
	}
	return nil
}

func (m *Manager) SetChatLabel(ctx context.Context, id, chat, labelID string, labeled bool) (LabelResult, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return LabelResult{}, err
	}
	jid, _, err := parseMessageRecipient(chat)
	if err != nil {
		return LabelResult{}, err
	}
	labelID, err = validateLabelID(labelID)
	if err != nil {
		return LabelResult{}, err
	}
	if err := current.client.SendAppState(ctx, appstate.BuildLabelChat(jid, labelID, labeled)); err != nil {
		return LabelResult{}, fmt.Errorf("update WhatsApp chat label: %w", err)
	}
	return LabelResult{LabelID: labelID, Target: jid.String(), Labeled: &labeled}, nil
}

func (m *Manager) SetMessageLabel(ctx context.Context, id, chat, messageID, labelID string, labeled bool) (LabelResult, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return LabelResult{}, err
	}
	jid, _, err := parseMessageRecipient(chat)
	if err != nil {
		return LabelResult{}, err
	}
	labelID, err = validateLabelID(labelID)
	if err != nil {
		return LabelResult{}, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" || len(messageID) > 200 {
		return LabelResult{}, errors.New("messageId is required and must have at most 200 characters")
	}
	if err := current.client.SendAppState(ctx, appstate.BuildLabelMessage(jid, labelID, messageID, labeled)); err != nil {
		return LabelResult{}, fmt.Errorf("update WhatsApp message label: %w", err)
	}
	return LabelResult{LabelID: labelID, Target: jid.String(), MessageID: messageID, Labeled: &labeled}, nil
}

func (m *Manager) EditLabel(ctx context.Context, id, labelID, name string, color int32, deleted bool) (LabelResult, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return LabelResult{}, err
	}
	labelID, err = validateLabelID(labelID)
	if err != nil {
		return LabelResult{}, err
	}
	name = strings.TrimSpace(name)
	if !deleted && name == "" {
		return LabelResult{}, errors.New("name is required when the label is not deleted")
	}
	if utf8.RuneCountInString(name) > 100 || color < 0 || color > 255 {
		return LabelResult{}, errors.New("name must have at most 100 characters and color must be between 0 and 255")
	}
	if err := current.client.SendAppState(ctx, appstate.BuildLabelEdit(labelID, name, color, deleted)); err != nil {
		return LabelResult{}, fmt.Errorf("edit WhatsApp label: %w", err)
	}
	return LabelResult{LabelID: labelID, Name: name, Color: color, Deleted: deleted}, nil
}

func (m *Manager) CreateCommunity(ctx context.Context, id, name string) (Group, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Group{}, err
	}
	name, err = validateGroupName(name)
	if err != nil {
		return Group{}, err
	}
	community, err := current.client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, GroupParent: types.GroupParent{IsParent: true}})
	if err != nil {
		return Group{}, fmt.Errorf("create WhatsApp community: %w", err)
	}
	return mapGroup(community), nil
}

func (m *Manager) UpdateCommunityGroups(ctx context.Context, id, communityJID string, groupJIDs []string, link bool) (CommunityGroupsResult, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return CommunityGroupsResult{}, err
	}
	community, err := parseGroupJID(communityJID)
	if err != nil {
		return CommunityGroupsResult{}, fmt.Errorf("invalid communityJid: %w", err)
	}
	if len(groupJIDs) == 0 || len(groupJIDs) > 100 {
		return CommunityGroupsResult{}, errors.New("groupJids must contain between 1 and 100 groups")
	}
	result := CommunityGroupsResult{Success: make([]string, 0, len(groupJIDs)), Failed: make(map[string]string)}
	for _, value := range groupJIDs {
		group, parseErr := parseGroupJID(value)
		if parseErr != nil {
			result.Failed[value] = parseErr.Error()
			continue
		}
		if link {
			err = current.client.LinkGroup(ctx, community, group)
		} else {
			err = current.client.UnlinkGroup(ctx, community, group)
		}
		if err != nil {
			result.Failed[group.String()] = err.Error()
		} else {
			result.Success = append(result.Success, group.String())
		}
	}
	if len(result.Failed) == 0 {
		result.Failed = nil
	}
	return result, nil
}

func validateNewsletter(name, description string) (string, string, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", "", errors.New("name is required and must have at most 100 characters")
	}
	if utf8.RuneCountInString(description) > 2048 {
		return "", "", errors.New("description must have at most 2048 characters")
	}
	return name, description, nil
}

func parseNewsletterJID(value string) (types.JID, error) {
	jid, err := types.ParseJID(strings.TrimSpace(value))
	if err != nil || jid.User == "" || jid.Server != types.NewsletterServer {
		return types.EmptyJID, errors.New("newsletterJid must be a valid @newsletter JID")
	}
	return jid.ToNonAD(), nil
}

func mapNewsletter(metadata *types.NewsletterMetadata) Newsletter {
	if metadata == nil {
		return Newsletter{}
	}
	result := Newsletter{JID: metadata.ID.String(), Name: metadata.ThreadMeta.Name.Text, Description: metadata.ThreadMeta.Description.Text,
		InviteCode: metadata.ThreadMeta.InviteCode, Subscribers: metadata.ThreadMeta.SubscriberCount, State: string(metadata.State.Type), Verification: string(metadata.ThreadMeta.VerificationState)}
	if metadata.ViewerMeta != nil {
		result.Role, result.Mute = string(metadata.ViewerMeta.Role), string(metadata.ViewerMeta.Mute)
	}
	return result
}

func validateLabelID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return "", errors.New("labelId is required and must have at most 128 characters")
	}
	return value, nil
}
