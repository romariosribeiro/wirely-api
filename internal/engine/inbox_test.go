package engine

import "testing"

func TestParseChatJID(t *testing.T) {
	for _, value := range []string{
		"5511999999999@s.whatsapp.net",
		"120363000000@g.us",
		"123456789@lid",
	} {
		jid, err := parseChatJID(value)
		if err != nil || jid.String() != value {
			t.Fatalf("parseChatJID(%q) = %q, %v", value, jid.String(), err)
		}
	}
}

func TestParseChatJIDRejectsUnsupportedTargets(t *testing.T) {
	for _, value := range []string{"", "status@broadcast", "channel@newsletter", "invalid"} {
		if _, err := parseChatJID(value); err == nil {
			t.Fatalf("parseChatJID(%q) should fail", value)
		}
	}
}

func TestFirstNotEmpty(t *testing.T) {
	if got := firstNotEmpty(" ", "Empresa", "Contato"); got != "Empresa" {
		t.Fatalf("unexpected value %q", got)
	}
}
