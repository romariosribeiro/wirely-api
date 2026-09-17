package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/webui"
)

const (
	sessionCookieName = "wirely_session"
	sessionDuration   = 24 * time.Hour
)

type MessageSender interface {
	SendText(context.Context, string, string, string) (engine.SentMessage, error)
	SendMedia(context.Context, string, string, engine.MediaPayload) (engine.SentMessage, error)
}

type StructuredMessageSender interface {
	SendLocation(context.Context, string, string, engine.LocationPayload) (engine.SentMessage, error)
	SendContact(context.Context, string, string, engine.ContactPayload) (engine.SentMessage, error)
	SendPoll(context.Context, string, string, engine.PollPayload) (engine.SentMessage, error)
	SendReaction(context.Context, string, string, engine.ReactionPayload) (engine.SentMessage, error)
}

type ChatPresenceSender interface {
	SendChatPresence(context.Context, string, string, string) error
}

type InstanceConnector interface {
	Connect(string) error
}

type InstanceJIDProvider interface {
	JID(string) string
}

type ContactProvider interface {
	Contacts(context.Context, string) ([]engine.Contact, error)
}

type ProfileManager interface {
	GetProfile(context.Context, string) (engine.Profile, error)
	UpdateProfile(context.Context, string, *string, *string) error
	SetProfilePhoto(context.Context, string, []byte) (string, error)
	GetPrivacySettings(context.Context, string) (engine.PrivacySettings, error)
	SetPrivacySetting(context.Context, string, string, string) (engine.PrivacySettings, error)
}

type GroupManager interface {
	ListGroups(context.Context, string) ([]engine.Group, error)
	GetGroup(context.Context, string, string) (engine.Group, error)
	CreateGroup(context.Context, string, string, []string) (engine.Group, error)
	SetGroupName(context.Context, string, string, string) error
	UpdateGroupParticipants(context.Context, string, string, string, []string) ([]engine.GroupParticipant, error)
	GroupInviteLink(context.Context, string, string, bool) (string, error)
	JoinGroup(context.Context, string, string) (string, error)
}

type ChatMessageSender interface {
	SendChatText(context.Context, string, string, string) (engine.SentMessage, error)
}

type ReceivedMediaProvider interface {
	OpenReceivedMedia(context.Context, string, string) (engine.ReceivedMedia, error)
}

type MessageActionManager interface {
	DeleteMessage(context.Context, string, string, string, string) (engine.MessageActionResult, error)
	EditMessage(context.Context, string, string, string, string) (engine.MessageActionResult, error)
	MarkMessagesRead(context.Context, string, string, []string, string) (engine.MessageActionResult, error)
	ArchiveChat(context.Context, string, string, bool) (engine.ChatActionResult, error)
	MuteChat(context.Context, string, string, time.Duration) (engine.ChatActionResult, error)
	PinChat(context.Context, string, string, bool) (engine.ChatActionResult, error)
}

type OrganizationManager interface {
	CreateNewsletter(context.Context, string, string, string) (engine.Newsletter, error)
	GetNewsletter(context.Context, string, string) (engine.Newsletter, error)
	GetNewsletterByInvite(context.Context, string, string) (engine.Newsletter, error)
	ListNewsletters(context.Context, string) ([]engine.Newsletter, error)
	GetNewsletterMessages(context.Context, string, string, int, int64) ([]engine.NewsletterMessage, error)
	SubscribeNewsletter(context.Context, string, string) error
	SetChatLabel(context.Context, string, string, string, bool) (engine.LabelResult, error)
	SetMessageLabel(context.Context, string, string, string, string, bool) (engine.LabelResult, error)
	EditLabel(context.Context, string, string, string, int32, bool) (engine.LabelResult, error)
	CreateCommunity(context.Context, string, string) (engine.Group, error)
	UpdateCommunityGroups(context.Context, string, string, []string, bool) (engine.CommunityGroupsResult, error)
}

type MessageQueue interface {
	EnqueueText(context.Context, string, string, string, time.Time, string) (storage.MessageJob, bool, error)
	EnqueueMedia(context.Context, string, string, engine.MediaPayload, time.Time, string) (storage.MessageJob, bool, error)
	Cancel(context.Context, string, string) (storage.MessageJob, error)
	Retry(context.Context, string, string) (storage.MessageJob, error)
	PurgeInstance(context.Context, string) error
}

type WebhookRetrier interface {
	Retry(engine.Event) error
}

type WebhookTester interface {
	Test(string) (string, error)
}

type Dependencies struct {
	Store          *storage.Store
	Engine         *engine.Manager
	Connector      InstanceConnector
	SecureCookies  bool
	Sender         MessageSender
	Structured     StructuredMessageSender
	Presence       ChatPresenceSender
	Groups         GroupManager
	Profile        ProfileManager
	Webhooks       WebhookRetrier
	Contacts       ContactProvider
	ChatSender     ChatMessageSender
	ReceivedMedia  ReceivedMediaProvider
	MessageActions MessageActionManager
	Organization   OrganizationManager
	Queue          MessageQueue
	RateLimit      int
	Backups        BackupManager
	Restart        func()
}

type Server struct {
	handler        http.Handler
	store          *storage.Store
	engine         *engine.Manager
	connector      InstanceConnector
	sender         MessageSender
	structured     StructuredMessageSender
	presence       ChatPresenceSender
	groups         GroupManager
	profile        ProfileManager
	webhooks       WebhookRetrier
	webhookTester  WebhookTester
	contacts       ContactProvider
	chatSender     ChatMessageSender
	receivedMedia  ReceivedMediaProvider
	messageActions MessageActionManager
	organization   OrganizationManager
	queue          MessageQueue
	secureCookies  bool
	startedAt      time.Time
	limiter        *rateLimiter
	backups        BackupManager
	restart        func()
	httpMetrics    *httpMetrics
}

func New(dependencies Dependencies) *Server {
	connector := dependencies.Connector
	if connector == nil && dependencies.Engine != nil {
		connector = dependencies.Engine
	}
	sender := dependencies.Sender
	structured := dependencies.Structured
	presence := dependencies.Presence
	groups := dependencies.Groups
	profile := dependencies.Profile
	if sender == nil && dependencies.Engine != nil {
		sender = dependencies.Engine
	}
	if structured == nil && dependencies.Engine != nil {
		structured = dependencies.Engine
	}
	if groups == nil && dependencies.Engine != nil {
		groups = dependencies.Engine
	}
	if profile == nil && dependencies.Engine != nil {
		profile = dependencies.Engine
	}
	if presence == nil && dependencies.Engine != nil {
		presence = dependencies.Engine
	}
	contacts := dependencies.Contacts
	chatSender := dependencies.ChatSender
	receivedMedia := dependencies.ReceivedMedia
	messageActions := dependencies.MessageActions
	organization := dependencies.Organization
	if dependencies.Engine != nil {
		if contacts == nil {
			contacts = dependencies.Engine
		}
		if chatSender == nil {
			chatSender = dependencies.Engine
		}
		if receivedMedia == nil {
			receivedMedia = dependencies.Engine
		}
		if messageActions == nil {
			messageActions = dependencies.Engine
		}
		if organization == nil {
			organization = dependencies.Engine
		}
	}
	webhookTester, _ := dependencies.Webhooks.(WebhookTester)
	server := &Server{store: dependencies.Store, engine: dependencies.Engine, connector: connector, sender: sender, structured: structured, presence: presence, groups: groups, profile: profile, webhooks: dependencies.Webhooks, webhookTester: webhookTester,
		contacts: contacts, chatSender: chatSender, receivedMedia: receivedMedia, messageActions: messageActions, organization: organization, queue: dependencies.Queue, secureCookies: dependencies.SecureCookies, startedAt: time.Now(),
		limiter: newRateLimiter(dependencies.RateLimit, time.Minute), backups: dependencies.Backups, restart: dependencies.Restart,
		httpMetrics: newHTTPMetrics()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("GET /metrics", server.prometheusMetrics)
	mux.HandleFunc("GET /openapi.json", openAPI)
	mux.HandleFunc("GET /api/openapi.json", openAPI)
	mux.HandleFunc("POST /api/auth/login", server.login)
	mux.HandleFunc("POST /api/send/text", server.publicSendTextMessage)
	mux.HandleFunc("POST /api/send/media", server.publicSendMediaMessage)
	mux.HandleFunc("POST /api/send/location", server.publicSendLocation)
	mux.HandleFunc("POST /api/send/contact", server.publicSendContact)
	mux.HandleFunc("POST /api/send/poll", server.publicSendPoll)
	mux.HandleFunc("POST /api/send/reaction", server.publicSendReaction)
	mux.HandleFunc("GET /api/messages/{messageID}/media", server.publicDownloadReceivedMedia)
	mux.HandleFunc("POST /api/messages/delete", server.publicDeleteMessage)
	mux.HandleFunc("POST /api/messages/edit", server.publicEditMessage)
	mux.HandleFunc("POST /api/messages/read", server.publicMarkMessagesRead)
	mux.HandleFunc("GET /api/messages/{messageID}/status", server.publicGetMessageStatus)
	mux.HandleFunc("POST /api/chats/archive", server.publicArchiveChat)
	mux.HandleFunc("POST /api/chats/mute", server.publicMuteChat)
	mux.HandleFunc("POST /api/chats/pin", server.publicPinChat)
	mux.HandleFunc("POST /api/chats/unpin", server.publicUnpinChat)
	mux.HandleFunc("POST /api/newsletters", server.publicCreateNewsletter)
	mux.HandleFunc("POST /api/newsletters/info", server.publicGetNewsletter)
	mux.HandleFunc("POST /api/newsletters/invite", server.publicGetNewsletterByInvite)
	mux.HandleFunc("GET /api/newsletters", server.publicListNewsletters)
	mux.HandleFunc("POST /api/newsletters/messages", server.publicGetNewsletterMessages)
	mux.HandleFunc("POST /api/newsletters/subscribe", server.publicSubscribeNewsletter)
	mux.HandleFunc("POST /api/labels/chat", server.publicAddChatLabel)
	mux.HandleFunc("DELETE /api/labels/chat", server.publicRemoveChatLabel)
	mux.HandleFunc("POST /api/labels/message", server.publicAddMessageLabel)
	mux.HandleFunc("DELETE /api/labels/message", server.publicRemoveMessageLabel)
	mux.HandleFunc("PATCH /api/labels/{labelID}", server.publicEditLabel)
	mux.HandleFunc("POST /api/communities", server.publicCreateCommunity)
	mux.HandleFunc("POST /api/communities/groups", server.publicAddCommunityGroups)
	mux.HandleFunc("DELETE /api/communities/groups", server.publicRemoveCommunityGroups)
	mux.HandleFunc("GET /api/instance", server.publicGetInstance)
	mux.HandleFunc("POST /api/instance/connect", server.publicConnectInstance)
	mux.HandleFunc("POST /api/instance/disconnect", server.publicDisconnectInstance)
	mux.HandleFunc("DELETE /api/instance/logout", server.publicLogoutInstance)
	mux.HandleFunc("POST /api/instance/pair", server.publicPairInstance)
	mux.HandleFunc("DELETE /api/instance/proxy", server.publicDeleteInstanceProxy)
	mux.HandleFunc("GET /api/instance/qr", server.publicInstanceQR)
	mux.HandleFunc("GET /api/instance/status", server.publicInstanceStatus)
	mux.HandleFunc("POST /api/contacts/check", server.publicCheckContacts)
	mux.HandleFunc("POST /api/webhook/test", server.publicTestWebhook)
	mux.HandleFunc("GET /api/webhook/deliveries", server.publicListWebhookDeliveries)
	mux.HandleFunc("GET /api/webhook/jobs/{eventID}", server.publicGetWebhookJob)
	mux.HandleFunc("GET /api/groups", server.publicListGroups)
	mux.HandleFunc("POST /api/groups", server.publicCreateGroup)
	mux.HandleFunc("POST /api/groups/join", server.publicJoinGroup)
	mux.HandleFunc("GET /api/groups/{groupJID}", server.publicGetGroup)
	mux.HandleFunc("PATCH /api/groups/{groupJID}", server.publicSetGroupName)
	mux.HandleFunc("POST /api/groups/{groupJID}/participants", server.publicUpdateGroupParticipants)
	mux.HandleFunc("GET /api/groups/{groupJID}/invite", server.publicGetGroupInvite)
	mux.HandleFunc("POST /api/groups/{groupJID}/invite", server.publicRotateGroupInvite)
	mux.HandleFunc("GET /api/profile", server.publicGetProfile)
	mux.HandleFunc("PATCH /api/profile", server.publicUpdateProfile)
	mux.HandleFunc("PUT /api/profile/photo", server.publicSetProfilePhoto)
	mux.HandleFunc("DELETE /api/profile/photo", server.publicDeleteProfilePhoto)
	mux.HandleFunc("GET /api/profile/privacy", server.publicGetPrivacySettings)
	mux.HandleFunc("PATCH /api/profile/privacy", server.publicSetPrivacySetting)
	mux.HandleFunc("POST /api/queue/text", server.publicQueueText)
	mux.HandleFunc("POST /api/queue/media", server.publicQueueMedia)
	mux.HandleFunc("GET /api/queue/{jobID}", server.publicGetMessageJob)
	mux.HandleFunc("DELETE /api/queue/{jobID}", server.publicCancelMessageJob)
	mux.HandleFunc("POST /api/queue/{jobID}/retry", server.publicRetryMessageJob)
	mux.Handle("GET /api/auth/me", server.requireAuth(http.HandlerFunc(server.me)))
	mux.Handle("POST /api/auth/logout", server.requireAuth(http.HandlerFunc(server.logout)))
	mux.Handle("PUT /api/auth/password", server.requireAuth(server.auditAction("auth.password.change", "user", "", http.HandlerFunc(server.changePassword))))
	mux.Handle("GET /api/users", server.requireRole(storage.RoleOwner, http.HandlerFunc(server.listUsers)))
	mux.Handle("GET /api/audit", server.requireRole(storage.RoleAdmin, http.HandlerFunc(server.listAudit)))
	mux.Handle("GET /api/backups", server.requireRole(storage.RoleOwner, http.HandlerFunc(server.listBackups)))
	mux.Handle("POST /api/backups", server.requireRole(storage.RoleOwner, server.auditAction("backup.create", "backup", "", http.HandlerFunc(server.createBackup))))
	mux.Handle("GET /api/backups/{backupID}/download", server.requireRole(storage.RoleOwner, http.HandlerFunc(server.downloadBackup)))
	mux.Handle("POST /api/backups/{backupID}/restore", server.requireRole(storage.RoleOwner, server.auditAction("backup.restore", "backup", "backupID", http.HandlerFunc(server.restoreBackup))))
	mux.Handle("DELETE /api/backups/{backupID}", server.requireRole(storage.RoleOwner, server.auditAction("backup.delete", "backup", "backupID", http.HandlerFunc(server.deleteBackup))))
	mux.Handle("POST /api/users", server.requireRole(storage.RoleOwner, server.auditAction("user.create", "user", "", http.HandlerFunc(server.createUser))))
	mux.Handle("PATCH /api/users/{userID}", server.requireRole(storage.RoleOwner, server.auditAction("user.update", "user", "userID", http.HandlerFunc(server.updateUser))))
	mux.Handle("PUT /api/users/{userID}/password", server.requireRole(storage.RoleOwner, server.auditAction("user.password.reset", "user", "userID", http.HandlerFunc(server.resetUserPassword))))
	mux.Handle("DELETE /api/users/{userID}", server.requireRole(storage.RoleOwner, server.auditAction("user.delete", "user", "userID", http.HandlerFunc(server.deleteUser))))
	mux.Handle("GET /api/metrics", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.metrics)))
	mux.Handle("GET /api/alerts", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listAlerts)))
	mux.Handle("GET /api/instances", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listInstances)))
	mux.Handle("POST /api/instances", server.requireRole(storage.RoleAdmin, server.auditAction("instance.create", "instance", "", http.HandlerFunc(server.createInstance))))
	mux.Handle("POST /api/instances/{id}/connect", server.requireRole(storage.RoleOperator, server.auditAction("instance.connect", "instance", "id", http.HandlerFunc(server.connectInstance))))
	mux.Handle("POST /api/instances/{id}/disconnect", server.requireRole(storage.RoleOperator, server.auditAction("instance.disconnect", "instance", "id", http.HandlerFunc(server.disconnectInstance))))
	mux.Handle("GET /api/instances/{id}/state", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.instanceState)))
	mux.Handle("GET /api/instances/{id}/qr", server.requireRole(storage.RoleOperator, http.HandlerFunc(server.instanceQR)))
	mux.Handle("GET /api/instances/{id}/token", server.requireRole(storage.RoleAdmin, http.HandlerFunc(server.getInstanceToken)))
	mux.Handle("POST /api/instances/{id}/token", server.requireRole(storage.RoleAdmin, server.auditAction("instance.token.rotate", "instance", "id", http.HandlerFunc(server.rotateInstanceToken))))
	mux.Handle("GET /api/instances/{id}/webhook", server.requireRole(storage.RoleAdmin, http.HandlerFunc(server.getInstanceWebhook)))
	mux.Handle("PUT /api/instances/{id}/webhook", server.requireRole(storage.RoleAdmin, server.auditAction("instance.webhook.update", "instance", "id", http.HandlerFunc(server.updateInstanceWebhook))))
	mux.Handle("GET /api/instances/{id}/events", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listActivityEvents)))
	mux.Handle("GET /api/instances/{id}/webhook-deliveries", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listWebhookDeliveries)))
	mux.Handle("POST /api/instances/{id}/webhook-deliveries/{deliveryID}/retry", server.requireRole(storage.RoleOperator, server.auditAction("webhook.retry", "instance", "id", http.HandlerFunc(server.retryWebhookDelivery))))
	mux.Handle("GET /api/instances/{id}/contacts", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listContacts)))
	mux.Handle("GET /api/instances/{id}/chats", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listChats)))
	mux.Handle("GET /api/instances/{id}/chats/{chat}/messages", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listChatMessages)))
	mux.Handle("POST /api/instances/{id}/chats/{chat}/messages", server.requireRole(storage.RoleOperator, server.auditAction("chat.message.send", "instance", "id", http.HandlerFunc(server.sendChatMessage))))
	mux.Handle("POST /api/instances/{id}/chats/{chat}/read", server.requireRole(storage.RoleViewer, server.auditAction("chat.read", "instance", "id", http.HandlerFunc(server.markChatRead))))
	mux.Handle("GET /api/instances/{id}/queue", server.requireRole(storage.RoleViewer, http.HandlerFunc(server.listMessageJobs)))
	mux.Handle("DELETE /api/instances/{id}/queue/{jobID}", server.requireRole(storage.RoleOperator, server.auditAction("queue.cancel", "instance", "id", http.HandlerFunc(server.cancelMessageJob))))
	mux.Handle("POST /api/instances/{id}/queue/{jobID}/retry", server.requireRole(storage.RoleOperator, server.auditAction("queue.retry", "instance", "id", http.HandlerFunc(server.retryMessageJob))))
	mux.Handle("DELETE /api/instances/{id}", server.requireRole(storage.RoleAdmin, server.auditAction("instance.delete", "instance", "id", http.HandlerFunc(server.deleteInstance))))
	mux.HandleFunc("/api/", server.apiNotFound)
	mux.Handle("/", spaHandler())
	server.handler = securityHeaders(server.observe(mux))
	return server
}

func (s *Server) ListenAndServe(address string) error {
	httpServer := &http.Server{
		Addr: address, Handler: s.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	return httpServer.ListenAndServe()
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok", "service": "wirely-api", "version": "0.9.0",
	})
}

func (s *Server) apiNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "API route not found")
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(payload.Username) == "" {
		payload.Username = "admin"
	}
	user, valid, lockedUntil, err := s.store.AuthenticateUserProtected(r.Context(), payload.Username, payload.Password, clientIP(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authentication failed")
		return
	}
	if !lockedUntil.IsZero() {
		writeLoginLocked(w, lockedUntil)
		_ = s.store.RecordAudit(r.Context(), storage.AuditEntry{Username: strings.ToLower(strings.TrimSpace(payload.Username)), Action: "auth.login", Status: http.StatusTooManyRequests, SourceIP: clientIP(r), Details: map[string]any{"result": "locked"}})
		return
	}
	if !valid {
		_ = s.store.RecordAudit(r.Context(), storage.AuditEntry{Username: strings.ToLower(strings.TrimSpace(payload.Username)), Action: "auth.login", Status: http.StatusUnauthorized, SourceIP: clientIP(r), Details: map[string]any{"result": "denied"}})
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, expiresAt, err := s.store.CreateUserSession(r.Context(), user.ID, sessionDuration)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setSessionCookie(w, token, expiresAt)
	_ = s.store.RecordAudit(r.Context(), storage.AuditEntry{UserID: user.ID, Username: user.Username, Action: "auth.login", Status: http.StatusOK, SourceIP: clientIP(r), Details: map[string]any{"result": "success"}})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_ = s.store.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	err := s.store.ChangeUserPassword(r.Context(), user.ID, payload.CurrentPassword, payload.NewPassword)
	switch {
	case err == nil:
	case errors.Is(err, storage.ErrInvalidAdminPassword):
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	case errors.Is(err, storage.ErrInvalidNewPassword):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	default:
		writeError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.store.ListInstances(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list instances")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": instances})
}

func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	instance, err := s.store.CreateInstance(r.Context(), payload.Name)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, instance)
}

func (s *Server) deleteInstance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine != nil {
		if err := s.engine.Delete(id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove WhatsApp session")
			return
		}
	}
	if s.queue != nil {
		if err := s.queue.PurgeInstance(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to remove queued media")
			return
		}
	}
	deleted, err := s.store.DeleteInstance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete instance")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) connectInstance(w http.ResponseWriter, r *http.Request) {
	if s.sender == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Connect(r.PathValue("id")); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "connecting"})
}

func (s *Server) disconnectInstance(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Disconnect(r.PathValue("id")); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) instanceState(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	state, err := s.engine.State(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) instanceQR(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	png, err := s.engine.QRCode(r.PathValue("id"))
	if errors.Is(err, engine.ErrQRUnavailable) {
		writeError(w, http.StatusNotFound, "QR code is not available")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render QR code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) publicSendTextMessage(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if ok {
		s.sendTextForInstance(w, r, instanceID)
	}
}

func (s *Server) authenticateInstanceRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	instance, valid, err := s.store.AuthenticateInstanceToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to authenticate token")
		return "", false
	}
	if !valid {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "invalid or missing bearer token")
		return "", false
	}
	allowed, remaining, reset := s.limiter.allow(instance.ID, time.Now().UTC())
	setRateLimitHeaders(w, s.limiter.limit, remaining, reset)
	if !allowed {
		writeRateLimitExceeded(w, reset)
		return "", false
	}
	return instance.ID, true
}

func (s *Server) getInstanceToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.store.GetInstanceToken(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load instance token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "regenerationRequired": token == ""})
}

func (s *Server) rotateInstanceToken(w http.ResponseWriter, r *http.Request) {
	token, err := s.store.RotateInstanceToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (s *Server) getInstanceWebhook(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetWebhookConfig(r.Context(), r.PathValue("id"))
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load webhook configuration")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) updateInstanceWebhook(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		URL          string    `json:"url"`
		Enabled      *bool     `json:"enabled"`
		Events       *[]string `json:"events"`
		RotateSecret bool      `json:"rotateSecret"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	webhookURL, err := validateWebhookURL(payload.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var config storage.WebhookConfig
	if payload.Enabled == nil && payload.Events == nil && !payload.RotateSecret {
		config, err = s.store.SetWebhook(r.Context(), r.PathValue("id"), webhookURL)
	} else {
		current, readErr := s.store.GetWebhookConfig(r.Context(), r.PathValue("id"))
		if errors.Is(readErr, storage.ErrInstanceNotFound) {
			writeError(w, 404, "instance not found")
			return
		}
		if readErr != nil {
			writeError(w, 500, "failed to load webhook configuration")
			return
		}
		enabled, events := current.Enabled, current.Events
		if payload.Enabled != nil {
			enabled = *payload.Enabled
		}
		if payload.Events != nil {
			events = *payload.Events
		}
		config, err = s.store.SaveWebhook(r.Context(), r.PathValue("id"), webhookURL, enabled, events, payload.RotateSecret)
	}
	if errors.Is(err, storage.ErrWebhookEvents) || errors.Is(err, storage.ErrWebhookURLRequired) {
		writeError(w, 422, err.Error())
		return
	}
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update webhook configuration")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func validateWebhookURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 2048 {
		return "", errors.New("webhook URL must have at most 2048 characters")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("webhook URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("webhook URL cannot contain credentials or a fragment")
	}
	return parsed.String(), nil
}

func (s *Server) sendTextForInstance(w http.ResponseWriter, r *http.Request, instanceID string) {
	if s.sender == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Recipient string       `json:"recipient"`
		Message   string       `json:"message"`
		Options   *sendOptions `json:"options,omitempty"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	stopPresence, ok := s.beginSendOptions(w, r, instanceID, payload.Recipient, payload.Options)
	if !ok {
		return
	}
	defer stopPresence()
	result, err := s.sender.SendText(r.Context(), instanceID, payload.Recipient, payload.Message)
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return s.requireRole(storage.RoleViewer, next)
}

func (s *Server) requireRole(required storage.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		user, valid, err := s.store.SessionUser(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate session")
			return
		}
		if !valid {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !storage.RoleAllows(user.Role, required) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}
		ctx := context.WithValue(r.Context(), authenticatedUserKey{}, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type authenticatedUserKey struct{}

func currentUser(r *http.Request) (storage.User, bool) {
	user, ok := r.Context().Value(authenticatedUserKey{}).(storage.User)
	return user, ok
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: int(sessionDuration.Seconds()), HttpOnly: true,
		Secure: s.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func spaHandler() http.Handler {
	dist, err := fs.Sub(webui.Files, "dist")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested == "." || requested == "" {
			requested = "index.html"
		}
		file, openErr := dist.Open(requested)
		if openErr == nil {
			info, statErr := file.Stat()
			_ = file.Close()
			if statErr == nil && !info.IsDir() {
				if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
					w.Header().Set("Content-Type", contentType)
				}
				http.ServeFileFS(w, r, dist, requested)
				return
			}
		}
		if !errors.Is(openErr, fs.ErrNotExist) && openErr != nil {
			http.Error(w, "failed to load panel", http.StatusInternalServerError)
			return
		}
		http.ServeFileFS(w, r, dist, "index.html")
	})
}
