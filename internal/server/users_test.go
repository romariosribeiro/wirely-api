package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func loginUser(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) == 0 {
		t.Fatalf("login %s failed: %d %s", username, response.Code, response.Body.String())
	}
	return response.Result().Cookies()[0]
}

func requestWithCookie(handler http.Handler, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestUserManagementAndRoleAuthorization(t *testing.T) {
	store := testStore(t)
	ownerPassword, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})
	ownerCookie := loginUser(t, app.Handler(), "admin", ownerPassword)

	createOperator := requestWithCookie(app.Handler(), http.MethodPost, "/api/v1/users",
		`{"username":"operator.one","password":"operator-password-123","role":"operator"}`, ownerCookie)
	if createOperator.Code != http.StatusCreated {
		t.Fatalf("operator creation failed: %d %s", createOperator.Code, createOperator.Body.String())
	}
	var operator storage.User
	if err := json.Unmarshal(createOperator.Body.Bytes(), &operator); err != nil {
		t.Fatal(err)
	}
	operatorCookie := loginUser(t, app.Handler(), operator.Username, "operator-password-123")
	me := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/auth/me", "", operatorCookie)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"role":"operator"`) {
		t.Fatalf("operator principal missing: %d %s", me.Code, me.Body.String())
	}
	if response := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/instances", "", operatorCookie); response.Code != http.StatusOK {
		t.Fatalf("operator cannot read instances: %d", response.Code)
	}
	if response := requestWithCookie(app.Handler(), http.MethodPost, "/api/v1/instances", `{"name":"Denied"}`, operatorCookie); response.Code != http.StatusForbidden {
		t.Fatalf("operator created an instance: %d %s", response.Code, response.Body.String())
	}
	if response := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/users", "", operatorCookie); response.Code != http.StatusForbidden {
		t.Fatalf("operator listed users: %d", response.Code)
	}

	createViewer := requestWithCookie(app.Handler(), http.MethodPost, "/api/v1/users",
		`{"username":"viewer.one","password":"viewer-password-123","role":"viewer"}`, ownerCookie)
	if createViewer.Code != http.StatusCreated {
		t.Fatalf("viewer creation failed: %d %s", createViewer.Code, createViewer.Body.String())
	}
	var viewer storage.User
	_ = json.Unmarshal(createViewer.Body.Bytes(), &viewer)
	viewerCookie := loginUser(t, app.Handler(), viewer.Username, "viewer-password-123")
	if response := requestWithCookie(app.Handler(), http.MethodPost, "/api/v1/instances/example/connect", "", viewerCookie); response.Code != http.StatusForbidden {
		t.Fatalf("viewer connected an instance: %d", response.Code)
	}

	update := requestWithCookie(app.Handler(), http.MethodPatch, "/api/v1/users/"+operator.ID,
		`{"role":"viewer","enabled":true}`, ownerCookie)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"role":"viewer"`) {
		t.Fatalf("operator role update failed: %d %s", update.Code, update.Body.String())
	}
	if response := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/auth/me", "", operatorCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("role change did not revoke session: %d", response.Code)
	}

	reset := requestWithCookie(app.Handler(), http.MethodPut, "/api/v1/users/"+viewer.ID+"/password",
		`{"password":"viewer-reset-password-456"}`, ownerCookie)
	if reset.Code != http.StatusNoContent {
		t.Fatalf("password reset failed: %d %s", reset.Code, reset.Body.String())
	}
	if response := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/auth/me", "", viewerCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("password reset did not revoke session: %d", response.Code)
	}
	_ = loginUser(t, app.Handler(), viewer.Username, "viewer-reset-password-456")

	users := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/users", "", ownerCookie)
	if users.Code != http.StatusOK || !strings.Contains(users.Body.String(), `"username":"admin"`) {
		t.Fatalf("owner cannot list team: %d %s", users.Code, users.Body.String())
	}
	var listed struct {
		Data []storage.User `json:"data"`
	}
	_ = json.Unmarshal(users.Body.Bytes(), &listed)
	ownerID := ""
	for _, user := range listed.Data {
		if user.Role == storage.RoleOwner {
			ownerID = user.ID
		}
	}
	if ownerID == "" {
		t.Fatal("owner missing from user list")
	}
	if response := requestWithCookie(app.Handler(), http.MethodDelete, "/api/v1/users/"+ownerID, "", ownerCookie); response.Code != http.StatusConflict {
		t.Fatalf("owner deletion was accepted: %d", response.Code)
	}
}
