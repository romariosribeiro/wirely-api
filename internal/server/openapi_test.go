package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPISpec(t *testing.T) {
	app := New(Dependencies{Store: testStore(t)})
	for _, path := range []string{"/openapi.json", "/api/openapi.json"} {
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
			"/metrics", "/api/auth/login", "/api/auth/me", "/api/auth/logout", "/api/auth/password",
			"/api/users", "/api/users/{userID}", "/api/users/{userID}/password",
			"/api/metrics", "/api/alerts", "/api/audit", "/api/backups",
			"/api/backups/{backupID}", "/api/backups/{backupID}/download", "/api/backups/{backupID}/restore",
			"/api/instances", "/api/instances/{id}", "/api/instances/{id}/connect", "/api/instances/{id}/disconnect",
			"/api/instances/{id}/state", "/api/instances/{id}/qr", "/api/instances/{id}/token", "/api/instances/{id}/webhook",
			"/api/instances/{id}/events", "/api/instances/{id}/webhook-deliveries", "/api/instances/{id}/webhook-deliveries/{deliveryID}/retry",
			"/api/instances/{id}/contacts", "/api/instances/{id}/chats", "/api/instances/{id}/chats/{chat}/messages",
			"/api/instances/{id}/chats/{chat}/read", "/api/instances/{id}/queue", "/api/instances/{id}/queue/{jobID}", "/api/instances/{id}/queue/{jobID}/retry"} {
			if _, ok := spec.Paths[endpoint]; !ok {
				t.Fatalf("missing documented endpoint %s", endpoint)
			}
		}
		for _, removed := range []string{"/api/send/image", "/api/send/audio", "/api/send/document", "/api/queue/image", "/api/queue/audio", "/api/queue/document"} {
			if _, ok := spec.Paths[removed]; ok {
				t.Fatalf("removed endpoint is still documented: %s", removed)
			}
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
