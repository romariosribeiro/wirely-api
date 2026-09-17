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

func TestLoginLockoutAndAudit(t *testing.T) {
	store := testStore(t)
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	for attempt := 1; attempt <= storage.LoginMaxFailures; attempt++ {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong-password"})
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		request.RemoteAddr = "203.0.113.10:50100"
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		want := http.StatusUnauthorized
		if attempt == storage.LoginMaxFailures {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d: expected %d, got %d: %s", attempt, want, response.Code, response.Body.String())
		}
	}

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	request.RemoteAddr = "203.0.113.10:50101"
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("locked login was accepted: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	request.RemoteAddr = "203.0.113.11:50101"
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("lockout leaked to a different source IP: %d %s", response.Code, response.Body.String())
	}
	cookie := response.Result().Cookies()[0]

	created := requestWithCookie(app.Handler(), http.MethodPost, "/api/users",
		`{"username":"audit.viewer","password":"audit-password-123","role":"viewer"}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("audited operation failed: %d %s", created.Code, created.Body.String())
	}
	audit := requestWithCookie(app.Handler(), http.MethodGet, "/api/audit?action=user.create", "", cookie)
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"action":"user.create"`) || strings.Contains(audit.Body.String(), "audit-password-123") {
		t.Fatalf("unexpected audit response: %d %s", audit.Code, audit.Body.String())
	}
}

func TestBearerRateLimitIsPerInstance(t *testing.T) {
	store := testStore(t)
	first, err := store.CreateInstance(context.Background(), "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateInstance(context.Background(), "Second")
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store, RateLimit: 2})

	send := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/send/text", strings.NewReader(`{"recipient":"+5511999999999","message":"test"}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response
	}
	if response := send(first.APIToken); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("first request failed unexpectedly: %d", response.Code)
	}
	if response := send(first.APIToken); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("second request failed unexpectedly: %d", response.Code)
	}
	limited := send(first.APIToken)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" || limited.Header().Get("RateLimit-Limit") != "2" {
		t.Fatalf("rate limit not enforced: %d %#v", limited.Code, limited.Header())
	}
	if response := send(second.APIToken); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("rate limit leaked across instances: %d", response.Code)
	}
}
