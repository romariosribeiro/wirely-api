package engine

import (
	"context"
	"strings"
	"testing"

	waCompanionReg "go.mau.fi/whatsmeow/proto/waCompanionReg"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
)

func TestConfigureDeviceIdentityUsesGoogleChrome(t *testing.T) {
	configureDeviceIdentity()
	if whatsmeowStore.DeviceProps.GetOs() != "Google Chrome" {
		t.Fatalf("unexpected device name: %q", whatsmeowStore.DeviceProps.GetOs())
	}
	if whatsmeowStore.DeviceProps.GetPlatformType() != waCompanionReg.DeviceProps_CHROME {
		t.Fatalf("unexpected platform: %s", whatsmeowStore.DeviceProps.GetPlatformType())
	}
}

func TestPairPhoneRejectsInvalidNumberBeforeConnecting(t *testing.T) {
	manager := &Manager{}
	for _, phone := range []string{"", "123", "01199999999", strings.Repeat("9", 16)} {
		if _, err := manager.PairPhone(context.Background(), "instance", phone); err == nil {
			t.Fatalf("invalid phone %q was accepted", phone)
		}
	}
}

func TestCheckContactsRequiresBoundedList(t *testing.T) {
	manager := &Manager{}
	if _, err := manager.CheckContacts(context.Background(), "instance", nil); err == nil {
		t.Fatal("empty list was accepted")
	}
	phones := make([]string, 101)
	if _, err := manager.CheckContacts(context.Background(), "instance", phones); err == nil {
		t.Fatal("list above 100 items was accepted")
	}
}
