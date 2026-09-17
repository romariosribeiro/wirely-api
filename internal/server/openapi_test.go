package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPISpec(t *testing.T) {
	app := New(Dependencies{Store: testStore(t)})
	for _, path := range []string{"/openapi.json", "/api/v1/openapi.json"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatal("wrong content type")
		}
		if response.Header().Get("Cache-Control") != "public, max-age=300" {
			t.Fatal("missing documentation cache policy")
		}
		var spec struct {
			OpenAPI    string         `json:"openapi"`
			Paths      map[string]any `json:"paths"`
			Components map[string]any `json:"components"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &spec); err != nil {
			t.Fatalf("invalid OpenAPI JSON: %v", err)
		}
		if spec.OpenAPI != "3.1.0" {
			t.Fatalf("unexpected OpenAPI version %q", spec.OpenAPI)
		}
		for _, endpoint := range []string{"/api/send/text", "/api/send/media", "/api/send/location", "/api/send/contact", "/api/send/poll", "/api/send/reaction", "/api/messages/{messageID}/media",
			"/api/messages/delete", "/api/messages/edit", "/api/messages/read", "/api/messages/{messageID}/status", "/api/chats/archive", "/api/chats/mute", "/api/chats/pin", "/api/chats/unpin",
			"/api/newsletters", "/api/newsletters/info", "/api/newsletters/invite", "/api/newsletters/messages", "/api/newsletters/subscribe",
			"/api/labels/chat", "/api/labels/message", "/api/labels/{labelID}", "/api/communities", "/api/communities/groups",
			"/api/instance", "/api/instance/connect", "/api/instance/disconnect", "/api/instance/logout", "/api/instance/pair", "/api/instance/proxy", "/api/instance/qr", "/api/instance/status", "/api/contacts/check",
			"/api/webhook/test", "/api/webhook/jobs/{eventID}", "/api/webhook/deliveries",
			"/api/groups", "/api/groups/join", "/api/groups/{groupJID}", "/api/groups/{groupJID}/participants", "/api/groups/{groupJID}/invite", "/api/profile", "/api/profile/photo", "/api/profile/privacy",
			"/api/queue/text", "/api/queue/media", "/api/queue/{jobID}", "/api/queue/{jobID}/retry",
			"/metrics", "/api/auth/login", "/api/v1/auth/me", "/api/v1/auth/logout", "/api/v1/auth/password",
			"/api/v1/users", "/api/v1/users/{userID}", "/api/v1/users/{userID}/password",
			"/api/v1/metrics", "/api/v1/alerts", "/api/v1/audit", "/api/v1/backups",
			"/api/v1/backups/{backupID}", "/api/v1/backups/{backupID}/download", "/api/v1/backups/{backupID}/restore",
			"/api/v1/instances", "/api/v1/instances/{id}", "/api/v1/instances/{id}/connect", "/api/v1/instances/{id}/disconnect",
			"/api/v1/instances/{id}/state", "/api/v1/instances/{id}/qr", "/api/v1/instances/{id}/token", "/api/v1/instances/{id}/webhook",
			"/api/v1/instances/{id}/events", "/api/v1/instances/{id}/webhook-deliveries", "/api/v1/instances/{id}/webhook-deliveries/{deliveryID}/retry",
			"/api/v1/instances/{id}/contacts", "/api/v1/instances/{id}/chats", "/api/v1/instances/{id}/chats/{chat}/messages",
			"/api/v1/instances/{id}/chats/{chat}/read", "/api/v1/instances/{id}/queue", "/api/v1/instances/{id}/queue/{jobID}", "/api/v1/instances/{id}/queue/{jobID}/retry"} {
			if _, ok := spec.Paths[endpoint]; !ok {
				t.Fatalf("missing documented endpoint %s", endpoint)
			}
		}
		for _, removed := range []string{"/api/send/image", "/api/send/audio", "/api/send/document", "/api/queue/image", "/api/queue/audio", "/api/queue/document"} {
			if _, ok := spec.Paths[removed]; ok {
				t.Fatalf("removed endpoint is still documented: %s", removed)
			}
		}
		if _, ok := spec.Paths["/api/v1/auth/login"]; ok {
			t.Fatal("removed login endpoint is still documented")
		}
		securitySchemes, ok := spec.Components["securitySchemes"].(map[string]any)
		if !ok || securitySchemes["BearerAuth"] == nil || securitySchemes["CookieAuth"] == nil {
			t.Fatal("missing security schemes")
		}
		parameters, ok := spec.Components["parameters"].(map[string]any)
		if !ok || parameters["IdempotencyKey"] == nil || parameters["MessageID"] == nil || parameters["EventID"] == nil {
			t.Fatal("missing Idempotency-Key parameter")
		}
		schemas, ok := spec.Components["schemas"].(map[string]any)
		if !ok || schemas["MessageJob"] == nil || schemas["QueueTextRequest"] == nil || schemas["User"] == nil || schemas["CreateUserRequest"] == nil ||
			schemas["LocationRequest"] == nil || schemas["ContactRequest"] == nil || schemas["PollRequest"] == nil || schemas["ReactionRequest"] == nil ||
			schemas["Group"] == nil || schemas["GroupParticipant"] == nil || schemas["CreateGroupRequest"] == nil || schemas["UpdateGroupParticipantsRequest"] == nil ||
			schemas["Profile"] == nil || schemas["PrivacySettings"] == nil || schemas["UpdateProfileRequest"] == nil || schemas["UpdatePrivacyRequest"] == nil ||
			schemas["Instance"] == nil || schemas["ConnectionState"] == nil || schemas["AuditEntry"] == nil || schemas["Backup"] == nil || schemas["Alert"] == nil ||
			schemas["ReceivedMediaMetadata"] == nil || schemas["ReceivedMediaBase64"] == nil || schemas["WebhookJob"] == nil || schemas["WebhookAccepted"] == nil ||
			schemas["PairInstanceRequest"] == nil || schemas["PairInstanceResponse"] == nil || schemas["ContactCheckRequest"] == nil || schemas["ContactCheck"] == nil ||
			schemas["DeleteMessageRequest"] == nil || schemas["EditMessageRequest"] == nil || schemas["MarkMessagesReadRequest"] == nil || schemas["MessageStatus"] == nil || schemas["ChatActionResult"] == nil {
			t.Fatal("missing queue or team schemas")
		}
	}
}
