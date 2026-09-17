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

func testStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestHealth(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestInstancesRequireAuthentication(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", response.Code)
	}
}

func TestShortAdminLoginEndpointCreatesSession(t *testing.T) {
	store := testStore(t)
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) == 0 {
		t.Fatalf("short login failed: %d %s", response.Code, response.Body.String())
	}
}

func TestCreateAndListInstances(t *testing.T) {
	store := testStore(t)
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	loginBody, _ := json.Marshal(map[string]string{"password": password})
	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login failed with status %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	createBody, _ := json.Marshal(map[string]string{"name": "Principal"})
	create := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewReader(createBody))
	create.AddCookie(cookies[0])
	createResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create failed with status %d: %s", createResponse.Code, createResponse.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/instances", nil)
	list.AddCookie(cookies[0])
	listResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), "Principal") {
		t.Fatalf("unexpected list response: %d %s", listResponse.Code, listResponse.Body.String())
	}
}
func TestSendTextRequiresAvailableEngine(t *testing.T) {
	store := testStore(t)
	instance, err := store.CreateInstance(context.Background(), "Principal")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	body, _ := json.Marshal(map[string]string{
		"recipient": "+5511999999999",
		"message":   "Teste",
	})
	request := httptest.NewRequest(http.MethodPost, "/api/send/text", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+instance.APIToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", response.Code, response.Body.String())
	}
}

func TestPublicSendTextRequiresBearerToken(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodPost, "/api/send/text", strings.NewReader(`{"recipient":"+5511999999999","message":"Teste"}`))
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatal("expected Bearer authentication challenge")
	}
}

func TestUnknownAPIRouteReturnsJSONNotFound(t *testing.T) {
	store := testStore(t)
	request := httptest.NewRequest(http.MethodPost, "/api/instances/example/messages/text", nil)
	response := httptest.NewRecorder()
	New(Dependencies{Store: store}).Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON response, got %q", response.Header().Get("Content-Type"))
	}
}

func TestChangePasswordRevokesSessions(t *testing.T) {
	store := testStore(t)
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	loginBody, _ := json.Marshal(map[string]string{"password": password})
	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login failed with status %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	shortBody, _ := json.Marshal(map[string]string{"currentPassword": password, "newPassword": "short"})
	shortChange := httptest.NewRequest(http.MethodPut, "/api/auth/password", bytes.NewReader(shortBody))
	shortChange.AddCookie(cookies[0])
	shortResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(shortResponse, shortChange)
	if shortResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("short password should be rejected, got status %d", shortResponse.Code)
	}

	wrongBody, _ := json.Marshal(map[string]string{"currentPassword": "incorrect-password", "newPassword": "another-secure-password"})
	wrongChange := httptest.NewRequest(http.MethodPut, "/api/auth/password", bytes.NewReader(wrongBody))
	wrongChange.AddCookie(cookies[0])
	wrongResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(wrongResponse, wrongChange)
	if wrongResponse.Code != http.StatusUnauthorized {
		t.Fatalf("incorrect current password should be rejected, got status %d", wrongResponse.Code)
	}

	stillValid := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	stillValid.AddCookie(cookies[0])
	stillValidResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(stillValidResponse, stillValid)
	if stillValidResponse.Code != http.StatusOK {
		t.Fatalf("failed password changes must preserve the session, got status %d", stillValidResponse.Code)
	}

	const newPassword = "a-new-secure-password"
	changeBody, _ := json.Marshal(map[string]string{"currentPassword": password, "newPassword": newPassword})
	change := httptest.NewRequest(http.MethodPut, "/api/auth/password", bytes.NewReader(changeBody))
	change.AddCookie(cookies[0])
	changeResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(changeResponse, change)
	if changeResponse.Code != http.StatusNoContent {
		t.Fatalf("password change failed with status %d: %s", changeResponse.Code, changeResponse.Body.String())
	}

	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(cookies[0])
	meResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old session should be revoked, got status %d", meResponse.Code)
	}

	oldLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	oldLoginResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(oldLoginResponse, oldLogin)
	if oldLoginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("old password should be rejected, got status %d", oldLoginResponse.Code)
	}

	newLoginBody, _ := json.Marshal(map[string]string{"password": newPassword})
	newLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(newLoginBody))
	newLoginResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(newLoginResponse, newLogin)
	if newLoginResponse.Code != http.StatusOK {
		t.Fatalf("new password should authenticate, got status %d: %s", newLoginResponse.Code, newLoginResponse.Body.String())
	}
}

func TestWebhookConfiguration(t *testing.T) {
	store := testStore(t)
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instance, err := store.CreateInstance(context.Background(), "Webhook")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	loginBody, _ := json.Marshal(map[string]string{"password": password})
	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookie := loginResponse.Result().Cookies()[0]
	endpoint := "/api/instances/" + instance.ID + "/webhook"

	get := httptest.NewRequest(http.MethodGet, endpoint, nil)
	get.AddCookie(cookie)
	getResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected initial config: %d %s", getResponse.Code, getResponse.Body.String())
	}

	invalid := httptest.NewRequest(http.MethodPut, endpoint, strings.NewReader(`{"url":"javascript:alert(1)"}`))
	invalid.AddCookie(cookie)
	invalidResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe URL should be rejected, got %d", invalidResponse.Code)
	}

	update := httptest.NewRequest(http.MethodPut, endpoint, strings.NewReader(`{"url":"https://example.com/wirely"}`))
	update.AddCookie(cookie)
	updateResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK || !strings.Contains(updateResponse.Body.String(), `"secret":"whsec_`) {
		t.Fatalf("unexpected webhook update: %d %s", updateResponse.Code, updateResponse.Body.String())
	}

	getAgain := httptest.NewRequest(http.MethodGet, endpoint, nil)
	getAgain.AddCookie(cookie)
	getAgainResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(getAgainResponse, getAgain)
	if getAgainResponse.Code != http.StatusOK || strings.Contains(getAgainResponse.Body.String(), `"secret"`) {
		t.Fatalf("stored secret must not be exposed: %d %s", getAgainResponse.Code, getAgainResponse.Body.String())
	}
	if !strings.Contains(getAgainResponse.Body.String(), `"hasSecret":true`) {
		t.Fatalf("configured webhook must report a secret: %s", getAgainResponse.Body.String())
	}

	disable := httptest.NewRequest(http.MethodPut, endpoint, strings.NewReader(`{"url":""}`))
	disable.AddCookie(cookie)
	disableResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(disableResponse, disable)
	if disableResponse.Code != http.StatusOK || !strings.Contains(disableResponse.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected disabled config: %d %s", disableResponse.Code, disableResponse.Body.String())
	}
}
