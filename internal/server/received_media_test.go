package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type fakeReceivedMediaProvider struct {
	path       string
	err        error
	instanceID string
	messageID  string
}

func (provider *fakeReceivedMediaProvider) OpenReceivedMedia(_ context.Context, instanceID, messageID string) (engine.ReceivedMedia, error) {
	provider.instanceID, provider.messageID = instanceID, messageID
	if provider.err != nil {
		return engine.ReceivedMedia{}, provider.err
	}
	file, err := os.Open(provider.path)
	if err != nil {
		return engine.ReceivedMedia{}, err
	}
	return engine.ReceivedMedia{File: file, Kind: "image", MIMEType: "image/png", FileName: "foto segura.png", Size: 7, SavedAt: time.Now()}, nil
}

func TestDownloadReceivedMediaAsFileAndBase64(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Received media")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "media.bin")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &fakeReceivedMediaProvider{path: path}
	app := New(Dependencies{Store: store, ReceivedMedia: provider})

	fileRequest := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/media", nil)
	fileRequest.Header.Set("Authorization", "Bearer "+instance.APIToken)
	fileResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(fileResponse, fileRequest)
	if fileResponse.Code != http.StatusOK || fileResponse.Body.String() != "content" || fileResponse.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("file download failed: %d %#v %s", fileResponse.Code, fileResponse.Header(), fileResponse.Body.String())
	}
	if provider.instanceID != instance.ID || provider.messageID != "MSG-1" || !strings.Contains(fileResponse.Header().Get("Content-Disposition"), "foto segura.png") {
		t.Fatalf("download was not scoped correctly: %#v", provider)
	}

	base64Request := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/media?format=base64", nil)
	base64Request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	base64Response := httptest.NewRecorder()
	app.Handler().ServeHTTP(base64Response, base64Request)
	var payload struct {
		Data      string `json:"data"`
		Size      int64  `json:"size"`
		MessageID string `json:"messageId"`
	}
	if err := json.Unmarshal(base64Response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil || string(decoded) != "content" || payload.Size != 7 || payload.MessageID != "MSG-1" {
		t.Fatalf("unexpected base64 response: %#v %v", payload, err)
	}
}

func TestDownloadReceivedMediaAuthenticationAndErrors(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Received media errors")
	if err != nil {
		t.Fatal(err)
	}
	provider := &fakeReceivedMediaProvider{err: engine.ErrReceivedMediaNotFound}
	app := New(Dependencies{Store: store, ReceivedMedia: provider})

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/media", nil)
	unauthorizedResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorizedResponse.Code)
	}

	notFound := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/media", nil)
	notFound.Header.Set("Authorization", "Bearer "+instance.APIToken)
	notFoundResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(notFoundResponse, notFound)
	if notFoundResponse.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFoundResponse.Code)
	}

	provider.err = errors.New("disk error")
	failure := httptest.NewRequest(http.MethodGet, "/api/messages/MSG-1/media", nil)
	failure.Header.Set("Authorization", "Bearer "+instance.APIToken)
	failureResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(failureResponse, failure)
	if failureResponse.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", failureResponse.Code)
	}
}
