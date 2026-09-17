package engine

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestValidateProfilePhoto(t *testing.T) {
	if err := ValidateProfilePhoto(nil); err != nil {
		t.Fatalf("nil must remove the photo: %v", err)
	}
	if err := ValidateProfilePhoto([]byte{0xff, 0xd8, 0xff, 0x01}); err != nil {
		t.Fatal(err)
	}
	for _, photo := range [][]byte{{}, []byte("PNG"), append([]byte{0xff, 0xd8, 0xff}, make([]byte, MaxProfilePhotoBytes)...)} {
		if err := ValidateProfilePhoto(photo); err == nil {
			t.Fatalf("invalid photo accepted: %d bytes", len(photo))
		}
	}
}

func TestPrivacySettingValidation(t *testing.T) {
	tests := []struct {
		name, value string
		want        types.PrivacySettingType
	}{
		{"groupAdd", "contacts", types.PrivacySettingTypeGroupAdd},
		{"lastSeen", "none", types.PrivacySettingTypeLastSeen},
		{"readReceipts", "all", types.PrivacySettingTypeReadReceipts},
		{"online", "match_last_seen", types.PrivacySettingTypeOnline},
		{"stickers", "contact_allowlist", types.PrivacySettingTypeStickers},
	}
	for _, test := range tests {
		setting, allowed, err := privacySetting(test.name)
		if err != nil || setting != test.want {
			t.Fatalf("%s: %v %q", test.name, err, setting)
		}
		if _, ok := allowed[test.value]; !ok {
			t.Fatalf("%q should be allowed for %s", test.value, test.name)
		}
	}
	if _, _, err := privacySetting("unknown"); err == nil {
		t.Fatal("unknown privacy setting accepted")
	}
}

func TestMapPrivacy(t *testing.T) {
	got := mapPrivacy(types.PrivacySettings{Profile: types.PrivacySettingContacts, Online: types.PrivacySettingMatchLastSeen})
	if got.Profile != "contacts" || got.Online != "match_last_seen" {
		t.Fatalf("unexpected privacy mapping: %#v", got)
	}
}
