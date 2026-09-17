package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadInstanceTokenRequiresAdmin(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	instance, err := s.CreateInstance(ctx, "Token test")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := s.CreateSession(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: s})
	endpoint := "/api/v1/instances/" + instance.ID + "/token"
	for _, bearer := range []string{"", instance.APIToken} {
		r := httptest.NewRequest(http.MethodGet, endpoint, nil)
		r.Header.Set("Authorization", "Bearer "+bearer)
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), instance.APIToken) {
			t.Fatal("only an admin session may read tokens")
		}
	}
	for _, path := range []string{endpoint, "/api/v1/instances/missing/token", "/api/v1/instances"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("tokens must not be cached")
		}
		if path == endpoint {
			var body struct {
				Token                string `json:"token"`
				RegenerationRequired bool   `json:"regenerationRequired"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || body.Token != instance.APIToken || body.RegenerationRequired {
				t.Fatal("admin must read current token")
			}
		} else if strings.Contains(path, "missing") {
			if w.Code != 404 {
				t.Fatal("unknown instance must be 404")
			}
		} else if strings.Contains(w.Body.String(), instance.APIToken) {
			t.Fatal("instance lists must not expose tokens")
		}
	}
}
