package engine

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestAdvancedTextContext(t *testing.T) {
	chat := types.NewJID("120363000000000000", types.GroupServer)
	message, err := textMessage("Veja https://example.com", chat, MessageOptions{
		ReplyTo:  &ReplyOptions{MessageID: "ABC", Participant: "5511999999999", Text: "original"},
		Mentions: []string{"5511888888888", "5511888888888"}, Forwarded: true, LinkPreview: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	extended := message.GetExtendedTextMessage()
	contextInfo := extended.GetContextInfo()
	if extended.GetMatchedText() != "https://example.com" || contextInfo.GetStanzaID() != "ABC" || !contextInfo.GetIsForwarded() || len(contextInfo.GetMentionedJID()) != 1 {
		t.Fatalf("unexpected advanced context: %#v", extended)
	}
}

func TestAdvancedTextValidation(t *testing.T) {
	private := types.NewJID("5511999999999", types.DefaultUserServer)
	group := types.NewJID("120363000000000000", types.GroupServer)
	tests := []struct {
		name    string
		chat    types.JID
		options MessageOptions
		want    string
	}{
		{"missing group participant", group, MessageOptions{ReplyTo: &ReplyOptions{MessageID: "ABC"}}, "participant"},
		{"participant in private", private, MessageOptions{ReplyTo: &ReplyOptions{MessageID: "ABC", Participant: "5511888888888"}}, "only supported"},
		{"preview without url", private, MessageOptions{LinkPreview: true}, "requires"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := textMessage("hello", test.chat, test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestViewOnceValidation(t *testing.T) {
	for _, kind := range []MediaKind{MediaAudio, MediaDocument, MediaSticker} {
		payload := MediaPayload{Kind: kind, Data: []byte("x"), MIMEType: "application/octet-stream", FileName: "file.bin", ViewOnce: true}
		if kind == MediaAudio {
			payload.MIMEType = "audio/ogg"
		}
		if kind == MediaSticker {
			payload.MIMEType = "image/webp"
			payload.FileName = ""
		}
		if err := ValidateMedia(&payload); err == nil || !strings.Contains(err.Error(), "viewOnce") {
			t.Fatalf("%s: got %v", kind, err)
		}
	}
}

func TestLiveLocationValidation(t *testing.T) {
	if err := ValidateLiveLocation(&LiveLocationPayload{Latitude: -23.5, Longitude: -46.6, Bearing: 359}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLiveLocation(&LiveLocationPayload{Latitude: 91, Longitude: 0}); err == nil {
		t.Fatal("expected invalid latitude")
	}
	if err := ValidateLiveLocation(&LiveLocationPayload{Latitude: 0, Longitude: 0, Bearing: 360}); err == nil {
		t.Fatal("expected invalid bearing")
	}
}
