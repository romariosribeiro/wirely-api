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
)

const MaxProfilePhotoBytes = 5 << 20

type Profile struct {
	JID        string `json:"jid"`
	Name       string `json:"name"`
	About      string `json:"about,omitempty"`
	PictureID  string `json:"pictureId,omitempty"`
	PictureURL string `json:"pictureUrl,omitempty"`
}

type PrivacySettings struct {
	GroupAdd     string `json:"groupAdd"`
	LastSeen     string `json:"lastSeen"`
	Status       string `json:"status"`
	Profile      string `json:"profile"`
	ReadReceipts string `json:"readReceipts"`
	CallAdd      string `json:"callAdd"`
	Online       string `json:"online"`
	Messages     string `json:"messages"`
	Defense      string `json:"defense"`
	Stickers     string `json:"stickers"`
}

func (m *Manager) GetProfile(ctx context.Context, id string) (Profile, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return Profile{}, err
	}
	if current.client.Store.ID == nil {
		return Profile{}, ErrNotConnected
	}
	ownJID := current.client.Store.ID.ToNonAD()
	profile := Profile{JID: ownJID.String(), Name: current.client.Store.PushName}
	infos, err := current.client.GetUserInfo(ctx, []types.JID{ownJID})
	if err != nil {
		return Profile{}, fmt.Errorf("get WhatsApp profile: %w", err)
	}
	for _, info := range infos {
		profile.About = info.Status
		profile.PictureID = info.PictureID
		break
	}
	picture, pictureErr := current.client.GetProfilePictureInfo(ctx, ownJID, &whatsmeow.GetProfilePictureParams{Preview: true})
	if pictureErr == nil && picture != nil {
		profile.PictureID = picture.ID
		profile.PictureURL = picture.URL
	} else if pictureErr != nil && !errors.Is(pictureErr, whatsmeow.ErrProfilePictureNotSet) && !errors.Is(pictureErr, whatsmeow.ErrProfilePictureUnauthorized) {
		return Profile{}, fmt.Errorf("get WhatsApp profile photo: %w", pictureErr)
	}
	return profile, nil
}

func (m *Manager) UpdateProfile(ctx context.Context, id string, name, about *string) error {
	current, err := m.connectedSession(id)
	if err != nil {
		return err
	}
	if name == nil && about == nil {
		return errors.New("name or about is required")
	}
	if name != nil {
		value := strings.TrimSpace(*name)
		if value == "" || utf8.RuneCountInString(value) > 25 {
			return errors.New("name must contain between 1 and 25 characters")
		}
		*name = value
	}
	if about != nil {
		value := strings.TrimSpace(*about)
		if utf8.RuneCountInString(value) > 139 {
			return errors.New("about must have at most 139 characters")
		}
		*about = value
	}
	if name != nil {
		if err := current.client.SendAppState(ctx, appstate.BuildSettingPushName(*name)); err != nil {
			return fmt.Errorf("set WhatsApp profile name: %w", err)
		}
		current.client.Store.PushName = *name
	}
	if about != nil {
		if err := current.client.SetStatusMessage(ctx, types.SetStatusInput{Text: about}); err != nil {
			return fmt.Errorf("set WhatsApp about: %w", err)
		}
	}
	return nil
}

func ValidateProfilePhoto(photo []byte) error {
	if photo == nil {
		return nil
	}
	if len(photo) == 0 {
		return errors.New("photo is required")
	}
	if len(photo) > MaxProfilePhotoBytes {
		return fmt.Errorf("photo must have at most %d MB", MaxProfilePhotoBytes>>20)
	}
	if len(photo) < 3 || photo[0] != 0xff || photo[1] != 0xd8 || photo[2] != 0xff {
		return errors.New("profile photo must be a JPEG image")
	}
	return nil
}

func (m *Manager) SetProfilePhoto(ctx context.Context, id string, photo []byte) (string, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return "", err
	}
	if err := ValidateProfilePhoto(photo); err != nil {
		return "", err
	}
	pictureID, err := current.client.SetGroupPhoto(ctx, types.EmptyJID, photo)
	if err != nil {
		return "", fmt.Errorf("set WhatsApp profile photo: %w", err)
	}
	return pictureID, nil
}

func (m *Manager) GetPrivacySettings(ctx context.Context, id string) (PrivacySettings, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return PrivacySettings{}, err
	}
	settings, err := current.client.TryFetchPrivacySettings(ctx, true)
	if err != nil {
		return PrivacySettings{}, fmt.Errorf("get WhatsApp privacy settings: %w", err)
	}
	return mapPrivacy(*settings), nil
}

func (m *Manager) SetPrivacySetting(ctx context.Context, id, setting, value string) (PrivacySettings, error) {
	current, err := m.connectedSession(id)
	if err != nil {
		return PrivacySettings{}, err
	}
	settingType, allowed, err := privacySetting(setting)
	if err != nil {
		return PrivacySettings{}, err
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := allowed[value]; !ok {
		return PrivacySettings{}, fmt.Errorf("value %q is not valid for %s", value, setting)
	}
	settings, err := current.client.SetPrivacySetting(ctx, settingType, types.PrivacySetting(value))
	if err != nil {
		return PrivacySettings{}, fmt.Errorf("set WhatsApp privacy setting: %w", err)
	}
	return mapPrivacy(settings), nil
}

func privacySetting(name string) (types.PrivacySettingType, map[string]struct{}, error) {
	common := values("all", "contacts", "contact_blacklist", "none")
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "groupadd", "group_add":
		return types.PrivacySettingTypeGroupAdd, common, nil
	case "lastseen", "last_seen", "last":
		return types.PrivacySettingTypeLastSeen, common, nil
	case "status":
		return types.PrivacySettingTypeStatus, common, nil
	case "profile":
		return types.PrivacySettingTypeProfile, common, nil
	case "readreceipts", "read_receipts":
		return types.PrivacySettingTypeReadReceipts, values("all", "none"), nil
	case "online":
		return types.PrivacySettingTypeOnline, values("all", "match_last_seen"), nil
	case "calladd", "call_add":
		return types.PrivacySettingTypeCallAdd, values("all", "known"), nil
	case "messages":
		return types.PrivacySettingTypeMessages, values("all", "contacts"), nil
	case "defense":
		return types.PrivacySettingTypeDefense, values("on_standard", "off"), nil
	case "stickers":
		return types.PrivacySettingTypeStickers, values("contacts", "contact_allowlist", "none"), nil
	default:
		return "", nil, errors.New("unknown privacy setting")
	}
}

func values(items ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
}

func mapPrivacy(settings types.PrivacySettings) PrivacySettings {
	return PrivacySettings{
		GroupAdd: string(settings.GroupAdd), LastSeen: string(settings.LastSeen), Status: string(settings.Status),
		Profile: string(settings.Profile), ReadReceipts: string(settings.ReadReceipts), CallAdd: string(settings.CallAdd),
		Online: string(settings.Online), Messages: string(settings.Messages), Defense: string(settings.Defense), Stickers: string(settings.Stickers),
	}
}
