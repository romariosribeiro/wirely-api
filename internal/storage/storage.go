package storage

import (
	"context"
	"crypto/cipher"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
	_ "modernc.org/sqlite"
)

const adminPasswordKey = "admin_password_hash"

var (
	ErrInvalidAdminPassword = errors.New("current password is incorrect")
	ErrInvalidNewPassword   = errors.New("new password must be different and contain between 12 and 128 characters")
	ErrInstanceNotFound     = errors.New("instance not found")
)

type Store struct {
	db          *sql.DB
	tokenCipher cipher.AEAD
}

type Instance struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Engine    string    `json:"engine"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	APIToken  string    `json:"apiToken,omitempty"`
	InstanceSettings
}

type InstanceSettings struct {
	AlwaysOnline  bool   `json:"alwaysOnline"`
	RejectCall    bool   `json:"rejectCall"`
	MsgRejectCall string `json:"msgRejectCall"`
	ReadMessages  bool   `json:"readMessages"`
	IgnoreGroups  bool   `json:"ignoreGroups"`
	IgnoreStatus  bool   `json:"ignoreStatus"`
}

type WebhookConfig struct {
	URL       string   `json:"url"`
	Enabled   bool     `json:"enabled"`
	HasSecret bool     `json:"hasSecret"`
	Secret    string   `json:"secret,omitempty"`
	Events    []string `json:"events"`
}

type WebhookTarget struct {
	URL     string
	Secret  string
	Enabled bool
	Events  []string
}

func Open(dataDirectory string) (*Store, error) {
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	databasePath, err := filepath.Abs(filepath.Join(dataDirectory, "wirely.db"))
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: databasePath}).String() +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.openTokenVault(dataDirectory); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('owner', 'admin', 'operator', 'viewer')),
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    last_login_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS login_attempts (
    username TEXT NOT NULL,
    source_ip TEXT NOT NULL,
    failures INTEGER NOT NULL DEFAULT 0,
    window_started_at INTEGER NOT NULL,
    locked_until INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY(username, source_ip)
);
CREATE INDEX IF NOT EXISTS login_attempts_updated_idx ON login_attempts(updated_at);
CREATE TABLE IF NOT EXISTS admin_audit (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    status INTEGER NOT NULL,
    source_ip TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS admin_audit_created_idx ON admin_audit(created_at DESC);
CREATE INDEX IF NOT EXISTS admin_audit_actor_idx ON admin_audit(user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    engine TEXT NOT NULL DEFAULT 'whatsmeow',
    status TEXT NOT NULL DEFAULT 'disconnected',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    api_token_hash TEXT UNIQUE,
    webhook_url TEXT NOT NULL DEFAULT '',
    webhook_secret TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS activity_events (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    occurred_at INTEGER NOT NULL,
    data_json TEXT NOT NULL,
    dedupe_key TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS activity_events_instance_time_idx
    ON activity_events(instance_id, occurred_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS activity_events_dedupe_idx
    ON activity_events(instance_id, event, dedupe_key) WHERE dedupe_key != '';
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    url TEXT NOT NULL,
    attempt INTEGER NOT NULL,
    status TEXT NOT NULL,
    http_status INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    manual INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS webhook_deliveries_instance_time_idx
    ON webhook_deliveries(instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS webhook_deliveries_event_idx
    ON webhook_deliveries(instance_id, event_id);
CREATE TABLE IF NOT EXISTS webhook_jobs (
    event_id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('queued', 'retrying', 'processing', 'delivered', 'dead')),
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    next_attempt_at INTEGER NOT NULL,
    last_http_status INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    manual INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS webhook_jobs_due_idx
    ON webhook_jobs(status, next_attempt_at, created_at);
CREATE INDEX IF NOT EXISTS webhook_jobs_instance_idx
    ON webhook_jobs(instance_id, updated_at DESC);
CREATE TABLE IF NOT EXISTS chat_messages (
    event_id TEXT PRIMARY KEY REFERENCES activity_events(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL DEFAULT '',
    event TEXT NOT NULL,
    chat TEXT NOT NULL,
    sender TEXT NOT NULL DEFAULT '',
    from_me INTEGER NOT NULL DEFAULT 0,
    is_group INTEGER NOT NULL DEFAULT 0,
    push_name TEXT NOT NULL DEFAULT '',
    message_type TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    timestamp INTEGER NOT NULL,
    data_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS chat_messages_instance_time_idx
    ON chat_messages(instance_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS chat_messages_chat_time_idx
    ON chat_messages(instance_id, chat, timestamp DESC);
CREATE TABLE IF NOT EXISTS chat_reads (
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    chat TEXT NOT NULL,
    read_at INTEGER NOT NULL,
    PRIMARY KEY(instance_id, chat)
);
CREATE TABLE IF NOT EXISTS message_jobs (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    recipient TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    media_path TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    scheduled_at INTEGER NOT NULL,
    next_attempt_at INTEGER NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    result_json TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS message_jobs_idempotency_idx
    ON message_jobs(instance_id, idempotency_key) WHERE idempotency_key != '';
CREATE INDEX IF NOT EXISTS message_jobs_due_idx
    ON message_jobs(status, next_attempt_at, created_at);
CREATE INDEX IF NOT EXISTS message_jobs_instance_time_idx
    ON message_jobs(instance_id, created_at DESC);
INSERT OR IGNORE INTO chat_messages
(event_id, instance_id, message_id, event, chat, sender, from_me, is_group, push_name, message_type, text, timestamp, data_json)
SELECT id, instance_id,
       COALESCE(CAST(json_extract(data_json, '$.id') AS TEXT), ''),
       event,
       CAST(json_extract(data_json, '$.chat') AS TEXT),
       COALESCE(CAST(json_extract(data_json, '$.from') AS TEXT), ''),
       CASE WHEN json_extract(data_json, '$.fromMe') THEN 1 ELSE 0 END,
       CASE WHEN json_extract(data_json, '$.isGroup') THEN 1 ELSE 0 END,
       COALESCE(CAST(json_extract(data_json, '$.pushName') AS TEXT), ''),
       COALESCE(CAST(json_extract(data_json, '$.type') AS TEXT), ''),
       COALESCE(CAST(json_extract(data_json, '$.text') AS TEXT), ''),
       occurred_at, data_json
FROM activity_events
WHERE event LIKE 'message.%'
  AND event != 'message.receipt'
  AND COALESCE(CAST(json_extract(data_json, '$.chat') AS TEXT), '') != '';
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("run database migrations: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE sessions ADD COLUMN user_id TEXT NOT NULL DEFAULT ''"); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("add session user column: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id)"); err != nil {
		return fmt.Errorf("create session user index: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE instances ADD COLUMN api_token_hash TEXT"); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
		return fmt.Errorf("add instance token column: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS instances_api_token_hash_idx ON instances(api_token_hash)"); err != nil {
		return fmt.Errorf("create instance token index: %w", err)
	}
	for _, migration := range []string{
		"ALTER TABLE instances ADD COLUMN api_token_ciphertext TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE instances ADD COLUMN webhook_url TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE instances ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE instances ADD COLUMN webhook_enabled INTEGER NOT NULL DEFAULT 1",
		`ALTER TABLE instances ADD COLUMN webhook_events TEXT NOT NULL DEFAULT '["messages","connection"]'`,
		"ALTER TABLE instances ADD COLUMN always_online INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE instances ADD COLUMN reject_call INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE instances ADD COLUMN msg_reject_call TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE instances ADD COLUMN read_messages INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE instances ADD COLUMN ignore_groups INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE instances ADD COLUMN ignore_status INTEGER NOT NULL DEFAULT 0",
	} {
		if _, err := s.db.ExecContext(ctx, migration); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("add instance webhook column: %w", err)
		}
	}

	return nil
}

func (s *Store) EnsureAdmin(ctx context.Context) (string, error) {
	return s.ensureOwner(ctx)
}

func (s *Store) AuthenticateAdmin(ctx context.Context, password string) (bool, error) {
	_, valid, err := s.AuthenticateUser(ctx, "admin", password)
	return valid, err
}

func (s *Store) ChangeAdminPassword(ctx context.Context, currentPassword, newPassword string) error {
	if _, err := s.ensureOwner(ctx); err != nil {
		return err
	}
	return s.ChangeUserPassword(ctx, ownerUserID, currentPassword, newPassword)
}

func (s *Store) CreateSession(ctx context.Context, duration time.Duration) (string, time.Time, error) {
	userID := ""
	if _, err := s.GetUser(ctx, ownerUserID); err == nil {
		userID = ownerUserID
	} else if !errors.Is(err, ErrUserNotFound) {
		return "", time.Time{}, err
	}
	return s.createSession(ctx, userID, duration)
}

func (s *Store) SessionValid(ctx context.Context, token string) (bool, error) {
	_, valid, err := s.SessionUser(ctx, token)
	return valid, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", security.TokenHash(token))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) PruneSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now().UTC().Unix())
	if err != nil {
		return fmt.Errorf("prune sessions: %w", err)
	}
	return nil
}

func (s *Store) ListInstances(ctx context.Context) ([]Instance, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, engine, status, created_at, updated_at,
       always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status
FROM instances ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	defer rows.Close()
	instances := make([]Instance, 0)
	for rows.Next() {
		var instance Instance
		var createdAt, updatedAt int64
		if err := rows.Scan(&instance.ID, &instance.Name, &instance.Engine, &instance.Status, &createdAt, &updatedAt,
			&instance.AlwaysOnline, &instance.RejectCall, &instance.MsgRejectCall, &instance.ReadMessages,
			&instance.IgnoreGroups, &instance.IgnoreStatus); err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instance.CreatedAt = time.Unix(createdAt, 0).UTC()
		instance.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (s *Store) CreateInstance(ctx context.Context, name string) (Instance, error) {
	return s.CreateInstanceWithSettings(ctx, name, InstanceSettings{})
}

func (s *Store) CreateInstanceWithSettings(ctx context.Context, name string, settings InstanceSettings) (Instance, error) {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 60 {
		return Instance{}, errors.New("instance name must contain between 2 and 60 characters")
	}
	settings.MsgRejectCall = strings.TrimSpace(settings.MsgRejectCall)
	if len([]rune(settings.MsgRejectCall)) > 1000 {
		return Instance{}, errors.New("msgRejectCall must have at most 1000 characters")
	}
	id, err := security.RandomToken(12)
	if err != nil {
		return Instance{}, err
	}
	token, err := newInstanceToken()
	if err != nil {
		return Instance{}, err
	}
	encrypted, err := s.encryptInstanceToken(id, token)
	if err != nil {
		return Instance{}, err
	}
	now := time.Now().UTC()
	instance := Instance{
		ID: id, Name: name, Engine: "whatsmeow", Status: "disconnected",
		CreatedAt: now, UpdatedAt: now, APIToken: token, InstanceSettings: settings,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO instances (id, name, engine, status, created_at, updated_at, api_token_hash, api_token_ciphertext,
                       always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		instance.ID, instance.Name, instance.Engine, instance.Status, now.Unix(), now.Unix(),
		security.TokenHash(token), encrypted, settings.AlwaysOnline, settings.RejectCall,
		settings.MsgRejectCall, settings.ReadMessages, settings.IgnoreGroups, settings.IgnoreStatus,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Instance{}, errors.New("an instance with this name already exists")
		}
		return Instance{}, fmt.Errorf("create instance: %w", err)
	}
	return instance, nil
}

func (s *Store) DeleteInstance(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM instances WHERE id = ?", id)
	if err != nil {
		return false, fmt.Errorf("delete instance: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read deleted rows: %w", err)
	}
	return count > 0, nil
}

func (s *Store) GetInstance(ctx context.Context, id string) (Instance, error) {
	var instance Instance
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, engine, status, created_at, updated_at,
       always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status
FROM instances WHERE id = ?`, id).Scan(
		&instance.ID, &instance.Name, &instance.Engine, &instance.Status, &createdAt, &updatedAt,
		&instance.AlwaysOnline, &instance.RejectCall, &instance.MsgRejectCall, &instance.ReadMessages,
		&instance.IgnoreGroups, &instance.IgnoreStatus,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Instance{}, ErrInstanceNotFound
		}
		return Instance{}, fmt.Errorf("get instance: %w", err)
	}
	instance.CreatedAt = time.Unix(createdAt, 0).UTC()
	instance.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return instance, nil
}

func (s *Store) GetInstanceSettings(ctx context.Context, id string) (InstanceSettings, error) {
	var settings InstanceSettings
	err := s.db.QueryRowContext(ctx, `
SELECT always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status
FROM instances WHERE id = ?`, id).Scan(
		&settings.AlwaysOnline, &settings.RejectCall, &settings.MsgRejectCall,
		&settings.ReadMessages, &settings.IgnoreGroups, &settings.IgnoreStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceSettings{}, ErrInstanceNotFound
	}
	if err != nil {
		return InstanceSettings{}, fmt.Errorf("get instance settings: %w", err)
	}
	return settings, nil
}

func (s *Store) UpdateInstanceSettings(ctx context.Context, id string, settings InstanceSettings) error {
	settings.MsgRejectCall = strings.TrimSpace(settings.MsgRejectCall)
	if len([]rune(settings.MsgRejectCall)) > 1000 {
		return errors.New("msgRejectCall must have at most 1000 characters")
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE instances SET always_online = ?, reject_call = ?, msg_reject_call = ?, read_messages = ?,
                     ignore_groups = ?, ignore_status = ?, updated_at = ?
WHERE id = ?`, settings.AlwaysOnline, settings.RejectCall, settings.MsgRejectCall, settings.ReadMessages,
		settings.IgnoreGroups, settings.IgnoreStatus, time.Now().UTC().Unix(), id)
	if err != nil {
		return fmt.Errorf("update instance settings: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read instance settings update: %w", err)
	}
	if count == 0 {
		return ErrInstanceNotFound
	}
	return nil
}

func (s *Store) UpdateInstanceStatus(ctx context.Context, id, status string) error {
	switch status {
	case "disconnected", "connecting", "qr", "connected", "error", "logged_out":
	default:
		return errors.New("invalid instance status")
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE instances SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now().UTC().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("update instance status: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated rows: %w", err)
	}
	if count == 0 {
		return errors.New("instance not found")
	}
	return nil
}
func (s *Store) AuthenticateInstanceToken(ctx context.Context, token string) (Instance, bool, error) {
	if !strings.HasPrefix(token, "wly_") || len(token) < 24 {
		return Instance{}, false, nil
	}
	var instance Instance
	var createdAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, engine, status, created_at, updated_at,
       always_online, reject_call, msg_reject_call, read_messages, ignore_groups, ignore_status
FROM instances WHERE api_token_hash = ?`, security.TokenHash(token)).Scan(
		&instance.ID, &instance.Name, &instance.Engine, &instance.Status, &createdAt, &updatedAt,
		&instance.AlwaysOnline, &instance.RejectCall, &instance.MsgRejectCall, &instance.ReadMessages,
		&instance.IgnoreGroups, &instance.IgnoreStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Instance{}, false, nil
	}
	if err != nil {
		return Instance{}, false, fmt.Errorf("authenticate instance token: %w", err)
	}
	instance.CreatedAt = time.Unix(createdAt, 0).UTC()
	instance.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return instance, true, nil
}

func (s *Store) RotateInstanceToken(ctx context.Context, id string) (string, error) {
	token, err := newInstanceToken()
	if err != nil {
		return "", err
	}
	encrypted, err := s.encryptInstanceToken(id, token)
	if err != nil {
		return "", err
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE instances SET api_token_hash = ?, api_token_ciphertext = ?, updated_at = ? WHERE id = ?",
		security.TokenHash(token), encrypted, time.Now().UTC().Unix(), id,
	)
	if err != nil {
		return "", fmt.Errorf("rotate instance token: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("read token update result: %w", err)
	}
	if count == 0 {
		return "", errors.New("instance not found")
	}

	return token, nil
}
func (s *Store) GetWebhookConfig(ctx context.Context, id string) (WebhookConfig, error) {
	var config WebhookConfig
	var secret, rawEvents string
	if err := s.db.QueryRowContext(ctx,
		"SELECT webhook_url, webhook_secret, webhook_enabled, webhook_events FROM instances WHERE id = ?", id,
	).Scan(&config.URL, &secret, &config.Enabled, &rawEvents); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebhookConfig{}, ErrInstanceNotFound
		}
		return WebhookConfig{}, fmt.Errorf("get webhook configuration: %w", err)
	}
	config.Enabled = config.Enabled && config.URL != ""
	if err := json.Unmarshal([]byte(rawEvents), &config.Events); err != nil {
		return WebhookConfig{}, err
	}
	if config.Events == nil {
		config.Events = []string{}
	}
	config.HasSecret = secret != ""
	return config, nil
}

func (s *Store) SetWebhook(ctx context.Context, id, webhookURL string) (WebhookConfig, error) {
	if webhookURL == "" {
		result, err := s.db.ExecContext(ctx,
			"UPDATE instances SET webhook_url = '', webhook_secret = '', webhook_enabled = 0, updated_at = ? WHERE id = ?",
			time.Now().UTC().Unix(), id,
		)
		if err != nil {
			return WebhookConfig{}, fmt.Errorf("disable webhook: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return WebhookConfig{}, fmt.Errorf("read webhook update result: %w", err)
		}
		if count == 0 {
			return WebhookConfig{}, ErrInstanceNotFound
		}
		return WebhookConfig{}, nil
	}

	secretValue, err := security.RandomToken(32)
	if err != nil {
		return WebhookConfig{}, err
	}
	secretValue = "whsec_" + secretValue
	result, err := s.db.ExecContext(ctx,
		"UPDATE instances SET webhook_url = ?, webhook_secret = ?, webhook_enabled = 1, updated_at = ? WHERE id = ?",
		webhookURL, secretValue, time.Now().UTC().Unix(), id,
	)
	if err != nil {
		return WebhookConfig{}, fmt.Errorf("configure webhook: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WebhookConfig{}, fmt.Errorf("read webhook update result: %w", err)
	}
	if count == 0 {
		return WebhookConfig{}, ErrInstanceNotFound
	}
	return WebhookConfig{URL: webhookURL, Enabled: true, HasSecret: true, Secret: secretValue}, nil
}

func (s *Store) GetWebhookTarget(ctx context.Context, id string) (WebhookTarget, error) {
	var target WebhookTarget
	var rawEvents string
	if err := s.db.QueryRowContext(ctx,
		"SELECT webhook_url, webhook_secret, webhook_enabled, webhook_events FROM instances WHERE id = ?", id,
	).Scan(&target.URL, &target.Secret, &target.Enabled, &rawEvents); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebhookTarget{}, ErrInstanceNotFound
		}
		return WebhookTarget{}, fmt.Errorf("get webhook target: %w", err)
	}
	if err := json.Unmarshal([]byte(rawEvents), &target.Events); err != nil {
		return WebhookTarget{}, err
	}
	return target, nil
}

func newInstanceToken() (string, error) {
	value, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	return "wly_" + value, nil
}
