package engine

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestValidateMedia(t *testing.T) {
	tests := []struct {
		name      string
		payload   MediaPayload
		wantError string
	}{
		{name: "image", payload: MediaPayload{Kind: MediaImage, Data: []byte("png"), MIMEType: "image/png; charset=binary", Caption: " ok "}},
		{name: "video", payload: MediaPayload{Kind: MediaVideo, Data: []byte("video"), MIMEType: "video/mp4", Caption: "clip"}},
		{name: "audio", payload: MediaPayload{Kind: MediaAudio, Data: []byte("ogg"), MIMEType: "application/ogg"}},
		{name: "sticker", payload: MediaPayload{Kind: MediaSticker, Data: []byte("webp"), MIMEType: "image/webp"}},
		{name: "document", payload: MediaPayload{Kind: MediaDocument, Data: []byte("pdf"), MIMEType: "application/pdf", FileName: "../report.pdf"}},
		{name: "empty", payload: MediaPayload{Kind: MediaImage, MIMEType: "image/png"}, wantError: "file is required"},
		{name: "wrong mime", payload: MediaPayload{Kind: MediaImage, Data: []byte("x"), MIMEType: "text/plain"}, wantError: "requires an image"},
		{name: "wrong video mime", payload: MediaPayload{Kind: MediaVideo, Data: []byte("x"), MIMEType: "image/png"}, wantError: "requires a video"},
		{name: "wrong sticker mime", payload: MediaPayload{Kind: MediaSticker, Data: []byte("x"), MIMEType: "image/png"}, wantError: "requires a WebP"},
		{name: "sticker caption", payload: MediaPayload{Kind: MediaSticker, Data: []byte("x"), MIMEType: "image/webp", Caption: "not supported"}, wantError: "does not support captions"},
		{name: "document filename", payload: MediaPayload{Kind: MediaDocument, Data: []byte("x"), MIMEType: "application/pdf"}, wantError: "file name"},
		{name: "voice format", payload: MediaPayload{Kind: MediaAudio, Data: []byte("mp3"), MIMEType: "audio/mpeg", Voice: true}, wantError: "OGG/Opus"},
		{name: "caption", payload: MediaPayload{Kind: MediaImage, Data: []byte("x"), MIMEType: "image/png", Caption: strings.Repeat("x", 1025)}, wantError: "1024"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateMedia(&test.payload)
			if test.wantError == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantError != "" && (err == nil || !strings.Contains(err.Error(), test.wantError)) {
				t.Fatalf("got %v, want error containing %q", err, test.wantError)
			}
			if test.name == "document" && test.payload.FileName != "report.pdf" {
				t.Fatalf("unsafe filename: %q", test.payload.FileName)
			}
		})
	}
}

func TestUploadedMessageFields(t *testing.T) {
	upload := whatsmeow.UploadResponse{URL: "url", DirectPath: "path", MediaKey: []byte{1}, FileSHA256: []byte{2}, FileEncSHA256: []byte{3}, FileLength: 42}
	image := uploadedMessage(MediaPayload{Kind: MediaImage, MIMEType: "image/png", Caption: "caption"}, upload).GetImageMessage()
	if image.GetURL() != "url" || image.GetCaption() != "caption" || image.GetFileLength() != 42 {
		t.Fatal("image upload fields were not copied")
	}
	video := uploadedMessage(MediaPayload{Kind: MediaVideo, MIMEType: "video/mp4", Caption: "clip"}, upload).GetVideoMessage()
	if video.GetMimetype() != "video/mp4" || video.GetCaption() != "clip" {
		t.Fatal("video fields were not copied")
	}
	sticker := uploadedMessage(MediaPayload{Kind: MediaSticker, MIMEType: "image/webp"}, upload).GetStickerMessage()
	if sticker.GetMimetype() != "image/webp" || sticker.GetURL() != "url" {
		t.Fatal("sticker fields were not copied")
	}
	audio := uploadedMessage(MediaPayload{Kind: MediaAudio, MIMEType: "audio/ogg", Voice: true}, upload).GetAudioMessage()
	if !audio.GetPTT() || audio.GetMimetype() != "audio/ogg" {
		t.Fatal("audio fields were not copied")
	}
	document := uploadedMessage(MediaPayload{Kind: MediaDocument, MIMEType: "application/pdf", FileName: "a.pdf", Caption: "doc"}, upload).GetDocumentMessage()
	if document.GetFileName() != "a.pdf" || document.GetCaption() != "doc" {
		t.Fatal("document fields were not copied")
	}
}
