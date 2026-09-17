package engine

import (
	"context"
	"strings"
	"testing"
)

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
