package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func TestPrometheusEndpointAndAlerts(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if _, err := store.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	viewer, err := store.CreateUser(ctx, "alerts.viewer", "alerts-viewer-password", storage.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store})

	health := httptest.NewRecorder()
	app.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	metrics := httptest.NewRecorder()
	app.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metrics.Code != http.StatusOK || !strings.HasPrefix(metrics.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("prometheus endpoint failed: %d %s", metrics.Code, metrics.Body.String())
	}
	for _, sample := range []string{"wirely_up 1", "wirely_uptime_seconds", "wirely_http_requests_total"} {
		if !strings.Contains(metrics.Body.String(), sample) {
			t.Fatalf("missing Prometheus sample %q: %s", sample, metrics.Body.String())
		}
	}

	unauthenticated := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/alerts", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("alerts did not require a session: %d", unauthenticated.Code)
	}
	cookie := loginUser(t, app.Handler(), viewer.Username, "alerts-viewer-password")
	request := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":[]`) {
		t.Fatalf("unexpected alerts response: %d %s", response.Code, response.Body.String())
	}
}
