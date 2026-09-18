package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const MaxUserLookup = 100

type UserAvatar struct {
	JID        string `json:"jid"`
	ID         string `json:"id,omitempty"`
	URL        string `json:"url,omitempty"`
	Type       string `json:"type,omitempty"`
	DirectPath string `json:"directPath,omitempty"`
}

type BlockedUser struct {
	JID   string `json:"jid"`
	Phone string `json:"phone,omitempty"`
}

type UserDetails struct {
	JID          string `json:"jid"`
	Phone        string `json:"phone"`
	Status       string `json:"status,omitempty"`
	PictureID    string `json:"pictureId,omitempty"`
	LID          string `json:"lid,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	FullName     string `json:"fullName,omitempty"`
	PushName     string `json:"pushName,omitempty"`
	BusinessName string `json:"businessName,omitempty"`
}

func (m *Manager) GetUserAvatar(ctx context.Context, id, number string, preview bool) (UserAvatar, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return UserAvatar{}, err
	}
	jid, _, err := resolveMessageRecipient(ctx, current.client, number)
	if err != nil {
		return UserAvatar{}, err
	}
	picture, err := current.client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: preview})
	if errors.Is(err, whatsmeow.ErrProfilePictureNotSet) || errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized) {
		return UserAvatar{JID: jid.String()}, nil
	}
	if err != nil {
		return UserAvatar{}, fmt.Errorf("get WhatsApp user avatar: %w", err)
	}
	if picture == nil {
		return UserAvatar{JID: jid.String()}, nil
	}
	return UserAvatar{JID: jid.String(), ID: picture.ID, URL: picture.URL, Type: picture.Type, DirectPath: picture.DirectPath}, nil
}

func (m *Manager) GetBlocklist(ctx context.Context, id string) ([]BlockedUser, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	list, err := current.client.GetBlocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("get WhatsApp block list: %w", err)
	}
	return mapBlockedUsers(list.JIDs), nil
}

func (m *Manager) SetContactBlocked(ctx context.Context, id, number string, blocked bool) ([]BlockedUser, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	jid, _, err := resolveMessageRecipient(ctx, current.client, number)
	if err != nil {
		return nil, err
	}
	action := events.BlocklistChangeActionUnblock
	if blocked {
		action = events.BlocklistChangeActionBlock
	}
	list, err := current.client.UpdateBlocklist(ctx, jid, action)
	if err != nil {
		return nil, fmt.Errorf("update WhatsApp block list: %w", err)
	}
	return mapBlockedUsers(list.JIDs), nil
}

func (m *Manager) GetUsers(ctx context.Context, id string, numbers []string) ([]UserDetails, error) {
	if len(numbers) == 0 || len(numbers) > MaxUserLookup {
		return nil, fmt.Errorf("provide between 1 and %d phone numbers", MaxUserLookup)
	}
	current, err := m.connectedSession(id)
	if err != nil {
		return nil, err
	}
	jids := make([]types.JID, 0, len(numbers))
	seen := make(map[string]struct{}, len(numbers))
	for _, number := range numbers {
		jid, _, parseErr := resolveMessageRecipient(ctx, current.client, number)
		if parseErr != nil {
			return nil, parseErr
		}
		jid = jid.ToNonAD()
		if _, exists := seen[jid.String()]; exists {
			continue
		}
		seen[jid.String()] = struct{}{}
		jids = append(jids, jid)
	}
	infos, err := current.client.GetUserInfo(ctx, jids)
	if err != nil {
		return nil, fmt.Errorf("get WhatsApp users: %w", err)
	}
	result := make([]UserDetails, 0, len(jids))
	for _, jid := range jids {
		info := infos[jid]
		item := UserDetails{JID: jid.String(), Phone: jid.User, Status: info.Status, PictureID: info.PictureID}
		if !info.LID.IsEmpty() {
			item.LID = info.LID.String()
		}
		if current.client.Store.Contacts != nil {
			contact, contactErr := current.client.Store.Contacts.GetContact(ctx, jid)
			if contactErr == nil && contact.Found {
				item.FirstName, item.FullName, item.PushName, item.BusinessName = contact.FirstName, contact.FullName, contact.PushName, contact.BusinessName
			}
		}
		result = append(result, item)
	}
	return result, nil
}

func mapBlockedUsers(jids []types.JID) []BlockedUser {
	result := make([]BlockedUser, 0, len(jids))
	for _, jid := range jids {
		jid = jid.ToNonAD()
		item := BlockedUser{JID: jid.String()}
		if jid.Server == types.DefaultUserServer {
			item.Phone = jid.User
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return strings.Compare(result[i].JID, result[j].JID) < 0 })
	return result
}
