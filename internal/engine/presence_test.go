package engine

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestChatPresence(t *testing.T) {
	tests := []struct {
		input string
		state types.ChatPresence
		media types.ChatPresenceMedia
	}{
		{"composing", types.ChatPresenceComposing, types.ChatPresenceMediaText},
		{"recording", types.ChatPresenceComposing, types.ChatPresenceMediaAudio},
		{"paused", types.ChatPresencePaused, types.ChatPresenceMediaText},
	}
	for _, test := range tests {
		state, media, err := chatPresence(test.input)
		if err != nil || state != test.state || media != test.media {
			t.Fatalf("chatPresence(%q) = %q, %q, %v", test.input, state, media, err)
		}
	}
	if _, _, err := chatPresence("online"); err == nil {
		t.Fatal("invalid chat presence accepted")
	}
}
