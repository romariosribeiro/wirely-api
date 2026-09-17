package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type GroupParticipant struct {
	JID          string `json:"jid"`
	Phone        string `json:"phone,omitempty"`
	LID          string `json:"lid,omitempty"`
	DisplayName  string `json:"displayName,omitempty"`
	IsAdmin      bool   `json:"isAdmin"`
	IsSuperAdmin bool   `json:"isSuperAdmin"`
	Error        int    `json:"error,omitempty"`
}

type Group struct {
	JID                  string             `json:"jid"`
	Name                 string             `json:"name"`
	Topic                string             `json:"topic,omitempty"`
	Owner                string             `json:"owner,omitempty"`
	CreatedAt            string             `json:"createdAt,omitempty"`
	IsLocked             bool               `json:"isLocked"`
	IsAnnounce           bool               `json:"isAnnounce"`
	IsCommunity          bool               `json:"isCommunity"`
	LinkedParentJID      string             `json:"linkedParentJid,omitempty"`
	JoinApprovalRequired bool               `json:"joinApprovalRequired"`
	ParticipantCount     int                `json:"participantCount"`
	Participants         []GroupParticipant `json:"participants"`
}

func (m *Manager) ListGroups(ctx context.Context, id string) ([]Group, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	groups, err := current.client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list WhatsApp groups: %w", err)
	}
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		result = append(result, mapGroup(group))
	}
	return result, nil
}

func (m *Manager) GetGroup(ctx context.Context, id, groupJID string) (Group, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Group{}, err
	}
	jid, err := parseGroupJID(groupJID)
	if err != nil {
		return Group{}, err
	}
	group, err := current.client.GetGroupInfo(ctx, jid)
	if err != nil {
		return Group{}, fmt.Errorf("get WhatsApp group: %w", err)
	}
	return mapGroup(group), nil
}

func (m *Manager) CreateGroup(ctx context.Context, id, name string, participants []string) (Group, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Group{}, err
	}
	name, err = validateGroupName(name)
	if err != nil {
		return Group{}, err
	}
	members, err := parseGroupParticipants(participants)
	if err != nil {
		return Group{}, err
	}
	group, err := current.client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: members})
	if err != nil {
		return Group{}, fmt.Errorf("create WhatsApp group: %w", err)
	}
	return mapGroup(group), nil
}

func (m *Manager) SetGroupName(ctx context.Context, id, groupJID, name string) error {
	current, err := m.connectedSession(id)
	if err != nil {
		return err
	}
	jid, err := parseGroupJID(groupJID)
	if err != nil {
		return err
	}
	name, err = validateGroupName(name)
	if err != nil {
		return err
	}
	if err := current.client.SetGroupName(ctx, jid, name); err != nil {
		return fmt.Errorf("set WhatsApp group name: %w", err)
	}
	return nil
}

func (m *Manager) UpdateGroupParticipants(ctx context.Context, id, groupJID, action string, participants []string) ([]GroupParticipant, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	jid, err := parseGroupJID(groupJID)
	if err != nil {
		return nil, err
	}
	members, err := parseGroupParticipants(participants)
	if err != nil {
		return nil, err
	}
	var change whatsmeow.ParticipantChange
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "add":
		change = whatsmeow.ParticipantChangeAdd
	case "remove":
		change = whatsmeow.ParticipantChangeRemove
	case "promote":
		change = whatsmeow.ParticipantChangePromote
	case "demote":
		change = whatsmeow.ParticipantChangeDemote
	default:
		return nil, errors.New("action must be add, remove, promote, or demote")
	}
	updated, err := current.client.UpdateGroupParticipants(ctx, jid, members, change)
	if err != nil {
		return nil, fmt.Errorf("%s WhatsApp group participants: %w", change, err)
	}
	result := make([]GroupParticipant, 0, len(updated))
	for _, participant := range updated {
		result = append(result, mapGroupParticipant(participant))
	}
	return result, nil
}

func (m *Manager) GroupInviteLink(ctx context.Context, id, groupJID string, reset bool) (string, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return "", err
	}
	jid, err := parseGroupJID(groupJID)
	if err != nil {
		return "", err
	}
	link, err := current.client.GetGroupInviteLink(ctx, jid, reset)
	if err != nil {
		return "", fmt.Errorf("get WhatsApp group invite link: %w", err)
	}
	return link, nil
}

func (m *Manager) JoinGroup(ctx context.Context, id, code string) (string, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return "", err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 512 {
		return "", errors.New("a valid invite code or link is required")
	}
	jid, err := current.client.JoinGroupWithLink(ctx, code)
	if err != nil {
		return "", fmt.Errorf("join WhatsApp group: %w", err)
	}
	return jid.String(), nil
}

func validateGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("group name is required")
	}
	if utf8.RuneCountInString(name) > 25 {
		return "", errors.New("group name must have at most 25 characters")
	}
	return name, nil
}

func parseGroupJID(value string) (types.JID, error) {
	jid, err := types.ParseJID(strings.TrimSpace(value))
	if err != nil || jid.User == "" || jid.Server != types.GroupServer {
		return types.EmptyJID, errors.New("groupJid must be a valid WhatsApp group JID")
	}
	return jid.ToNonAD(), nil
}

func parseGroupParticipants(values []string) ([]types.JID, error) {
	if len(values) == 0 || len(values) > 1023 {
		return nil, errors.New("participants must contain between 1 and 1023 items")
	}
	result := make([]types.JID, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		jid, _, err := parseMessageRecipient(value)
		if err != nil || jid.Server == types.GroupServer {
			return nil, fmt.Errorf("invalid participant %q", value)
		}
		key := jid.String()
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate participant %q", value)
		}
		seen[key] = struct{}{}
		result = append(result, jid)
	}
	return result, nil
}

func mapGroup(group *types.GroupInfo) Group {
	if group == nil {
		return Group{Participants: []GroupParticipant{}}
	}
	participants := make([]GroupParticipant, 0, len(group.Participants))
	for _, participant := range group.Participants {
		participants = append(participants, mapGroupParticipant(participant))
	}
	count := group.ParticipantCount
	if count == 0 {
		count = len(participants)
	}
	result := Group{
		JID: group.JID.String(), Name: group.Name, Topic: group.Topic, Owner: jidString(group.OwnerJID),
		IsLocked: group.IsLocked, IsAnnounce: group.IsAnnounce, IsCommunity: group.IsParent,
		LinkedParentJID: jidString(group.LinkedParentJID), JoinApprovalRequired: group.IsJoinApprovalRequired,
		ParticipantCount: count, Participants: participants,
	}
	if !group.GroupCreated.IsZero() {
		result.CreatedAt = group.GroupCreated.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return result
}

func mapGroupParticipant(participant types.GroupParticipant) GroupParticipant {
	return GroupParticipant{
		JID: jidString(participant.JID), Phone: jidString(participant.PhoneNumber), LID: jidString(participant.LID),
		DisplayName: participant.DisplayName, IsAdmin: participant.IsAdmin, IsSuperAdmin: participant.IsSuperAdmin, Error: participant.Error,
	}
}

func jidString(jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}
	return jid.ToNonAD().String()
}
