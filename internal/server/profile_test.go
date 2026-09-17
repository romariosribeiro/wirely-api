package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

type recordingProfileManager struct {
	instanceID string
	name       *string
	about      *string
	photo      []byte
	setting    string
	value      string
}

func (p *recordingProfileManager) GetProfile(_ context.Context, id string) (engine.Profile, error) {
	p.instanceID = id
	return engine.Profile{JID: "5511999999999@s.whatsapp.net", Name: "Wirely", About: "Online"}, nil
}
func (p *recordingProfileManager) UpdateProfile(_ context.Context, id string, name, about *string) error {
	p.instanceID, p.name, p.about = id, name, about
	return nil
}
func (p *recordingProfileManager) SetProfilePhoto(_ context.Context, id string, photo []byte) (string, error) {
	p.instanceID, p.photo = id, photo
	return "picture-id", nil
}
func (p *recordingProfileManager) GetPrivacySettings(_ context.Context, id string) (engine.PrivacySettings, error) {
	p.instanceID = id
	return engine.PrivacySettings{Profile: "contacts", Online: "match_last_seen"}, nil
}
func (p *recordingProfileManager) SetPrivacySetting(_ context.Context, id, setting, value string) (engine.PrivacySettings, error) {
	p.instanceID, p.setting, p.value = id, setting, value
	return engine.PrivacySettings{Profile: value}, nil
}

func profilePhotoRequest(t *testing.T, token string, photo []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "profile.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(photo); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/profile/photo", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestProfileEndpoints(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Profile")
	if err != nil {
		t.Fatal(err)
	}
	profile := &recordingProfileManager{}
	app := New(Dependencies{Store: store, Profile: profile})
	tests := []struct {
		method, path, body string
		status             int
		check              func() bool
	}{
		{http.MethodGet, "/api/profile", "", 200, func() bool { return profile.instanceID == instance.ID }},
		{http.MethodPatch, "/api/profile", `{"name":"Maria","about":"Disponível"}`, 204, func() bool { return profile.name != nil && *profile.name == "Maria" && profile.about != nil }},
		{http.MethodGet, "/api/profile/privacy", "", 200, func() bool { return profile.instanceID == instance.ID }},
		{http.MethodPatch, "/api/profile/privacy", `{"setting":"profile","value":"contacts"}`, 200, func() bool { return profile.setting == "profile" && profile.value == "contacts" }},
		{http.MethodDelete, "/api/profile/photo", "", 204, func() bool { return profile.photo == nil }},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
		request.Header.Set("Authorization", "Bearer "+instance.APIToken)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != test.status || !test.check() {
			t.Fatalf("%s %s: %d %s", test.method, test.path, response.Code, response.Body.String())
		}
	}

	photo := []byte{0xff, 0xd8, 0xff, 0x01}
	photoResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(photoResponse, profilePhotoRequest(t, instance.APIToken, photo))
	if photoResponse.Code != http.StatusOK || !bytes.Equal(profile.photo, photo) {
		t.Fatalf("photo update failed: %d %s", photoResponse.Code, photoResponse.Body.String())
	}
}

func TestProfilePhotoRejectsNonJPEG(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Profile")
	if err != nil {
		t.Fatal(err)
	}
	profile := &recordingProfileManager{}
	response := httptest.NewRecorder()
	New(Dependencies{Store: store, Profile: profile}).Handler().ServeHTTP(response, profilePhotoRequest(t, instance.APIToken, []byte("PNG")))
	if response.Code != http.StatusUnprocessableEntity || profile.photo != nil {
		t.Fatalf("non-JPEG accepted: %d %s", response.Code, response.Body.String())
	}
}
