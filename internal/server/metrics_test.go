package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func TestMetricsAPIReportsOperationalSnapshot(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if _, err := store.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	viewer, err := store.CreateUser(ctx, "metrics.viewer", "metrics-viewer-password", storage.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := store.CreateInstance(ctx, "Metrics API")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateInstanceStatus(ctx, instance.ID, "connected"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := store.SaveActivityEvent(ctx, "metrics-api-event", instance.ID, "message.received", now.Add(-time.Minute).Format(time.RFC3339Nano), map[string]any{"id": "message-1"}); err != nil {
		t.Fatal(err)
	}

	app := New(Dependencies{Store: store})
	cookie := loginUser(t, app.Handler(), viewer.Username, "metrics-viewer-password")
	request := httptest.NewRequest(http.MethodGet, "/api/metrics?range=24h", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("metrics failed: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("metrics may be cached: %q", response.Header().Get("Cache-Control"))
	}
	var body metricsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Range != "24h" || body.Instances.Total != 1 || body.Instances.Connected != 1 || body.Messages.Received != 1 {
		t.Fatalf("unexpected metrics response: %#v", body)
	}
	if body.UptimeSeconds < 0 || len(body.ByInstance) != 1 || body.ByInstance[0].ID != instance.ID {
		t.Fatalf("unexpected metric metadata: %#v", body)
	}
}

func TestMetricsAPIValidatesRangeAndAuthentication(t *testing.T) {
	store := testStore(t)
	app := New(Dependencies{Store: store})

	unauthenticated := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthenticated.Code)
	}

	cookie := authenticatedCookie(t, store, app.Handler())
	invalid := httptest.NewRequest(http.MethodGet, "/api/metrics?range=1y", nil)
	invalid.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, invalid)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "24h, 7d, or 30d") {
		t.Fatalf("invalid range accepted: %d %s", response.Code, response.Body.String())
	}
}
