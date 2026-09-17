package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/romariosribeiro/wirely-api/internal/security"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"

	ownerUserID = "usr_owner"
)

var (
	ErrUserNotFound        = errors.New("user not found")
	ErrUsernameUnavailable = errors.New("username is already in use")
	ErrInvalidUsername     = errors.New("username must contain 3 to 40 letters, numbers, dots, hyphens, or underscores")
	ErrInvalidUserRole     = errors.New("invalid user role")
	ErrOwnerImmutable      = errors.New("the owner account cannot be changed or deleted")
	ErrUserDisabled        = errors.New("user account is disabled")
)

type User struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Role        Role       `json:"role"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

func RoleAllows(role, required Role) bool {
	return roleLevel(role) >= roleLevel(required) && roleLevel(required) > 0
}

func roleLevel(role Role) int {
	switch role {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

func validAssignableRole(role Role) bool {
	return role == RoleAdmin || role == RoleOperator || role == RoleViewer
}

func normalizeUsername(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if length := len([]rune(value)); length < 3 || length > 40 {
		return "", ErrInvalidUsername
	}
	for _, character := range value {
		if character > unicode.MaxASCII || !(unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character)) {
			return "", ErrInvalidUsername
		}
	}
	return value, nil
}

func validateNewPassword(value string) error {
	if length := len([]rune(value)); length < 12 || length > 128 {
		return ErrInvalidNewPassword
	}
	return nil
}

func (s *Store) ensureOwner(ctx context.Context) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("start owner migration: %w", err)
	}
	defer tx.Rollback()

	var encoded string
	generatedPassword := ""
	err = tx.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", adminPasswordKey).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		generatedPassword, err = security.RandomToken(18)
		if err != nil {
			return "", err
		}
		encoded, err = security.HashPassword(generatedPassword)
		if err != nil {
			return "", err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)",
			adminPasswordKey, encoded, time.Now().UTC().Unix()); err != nil {
			return "", fmt.Errorf("create owner credentials: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("read owner credentials: %w", err)
	}

	now := time.Now().UTC().Unix()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO users
(id, username, password_hash, role, enabled, created_at, updated_at)
VALUES (?, 'admin', ?, 'owner', 1, ?, ?)`, ownerUserID, encoded, now, now); err != nil {
		return "", fmt.Errorf("create owner account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET username = 'admin', password_hash = ?, role = 'owner', enabled = 1, updated_at = ? WHERE id = ?`, encoded, now, ownerUserID); err != nil {
		return "", fmt.Errorf("protect owner account: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET user_id = ? WHERE user_id = ''", ownerUserID); err != nil {
		return "", fmt.Errorf("migrate owner sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit owner migration: %w", err)
	}
	return generatedPassword, nil
}

func (s *Store) AuthenticateUser(ctx context.Context, username, password string) (User, bool, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, false, nil
	}
	var user User
	var encoded string
	var enabled int
	var createdAt, updatedAt, lastLoginAt int64
	err = s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, role, enabled, created_at, updated_at, last_login_at
FROM users WHERE username = ?`, username).Scan(&user.ID, &user.Username, &encoded, &user.Role, &enabled, &createdAt, &updatedAt, &lastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("read user credentials: %w", err)
	}
	if enabled == 0 || !security.CheckPassword(encoded, password) {
		return User{}, false, nil
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, "UPDATE users SET last_login_at = ? WHERE id = ?", now.Unix(), user.ID); err != nil {
		return User{}, false, fmt.Errorf("record user login: %w", err)
	}
	user.Enabled = true
	user.CreatedAt = time.Unix(createdAt, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	user.LastLoginAt = &now
	return user, true, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, username, role, enabled, created_at, updated_at, last_login_at
FROM users ORDER BY CASE role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'operator' THEN 2 ELSE 3 END, username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT id, username, role, enabled, created_at, updated_at, last_login_at
FROM users WHERE id = ?`, id))
}

func scanUser(scanner interface{ Scan(...any) error }) (User, error) {
	var user User
	var enabled int
	var createdAt, updatedAt, lastLoginAt int64
	if err := scanner.Scan(&user.ID, &user.Username, &user.Role, &enabled, &createdAt, &updatedAt, &lastLoginAt); errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	} else if err != nil {
		return User{}, fmt.Errorf("scan user: %w", err)
	}
	user.Enabled = enabled != 0
	user.CreatedAt = time.Unix(createdAt, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if lastLoginAt > 0 {
		value := time.Unix(lastLoginAt, 0).UTC()
		user.LastLoginAt = &value
	}
	return user, nil
}

func (s *Store) CreateUser(ctx context.Context, username, password string, role Role) (User, error) {
	username, err := normalizeUsername(username)
	if err != nil {
		return User{}, err
	}
	if err := validateNewPassword(password); err != nil {
		return User{}, err
	}
	if !validAssignableRole(role) {
		return User{}, ErrInvalidUserRole
	}
	encoded, err := security.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	random, err := security.RandomToken(12)
	if err != nil {
		return User{}, err
	}
	id := "usr_" + random
	now := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(ctx, `INSERT INTO users
(id, username, password_hash, role, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, username, encoded, role, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return User{}, ErrUsernameUnavailable
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return s.GetUser(ctx, id)
}

func (s *Store) UpdateUser(ctx context.Context, id string, role Role, enabled bool) (User, error) {
	if id == ownerUserID {
		return User{}, ErrOwnerImmutable
	}
	if !validAssignableRole(role) {
		return User{}, ErrInvalidUserRole
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE users SET role = ?, enabled = ?, updated_at = ? WHERE id = ?",
		role, boolInt(enabled), time.Now().UTC().Unix(), id)
	if err != nil {
		return User{}, fmt.Errorf("update user: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return User{}, ErrUserNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return User{}, fmt.Errorf("revoke user sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.GetUser(ctx, id)
}

func (s *Store) ResetUserPassword(ctx context.Context, id, password string) error {
	if err := validateNewPassword(password); err != nil {
		return err
	}
	encoded, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	return s.setUserPassword(ctx, id, encoded)
}

func (s *Store) ChangeUserPassword(ctx context.Context, id, currentPassword, newPassword string) error {
	if err := validateNewPassword(newPassword); err != nil || currentPassword == newPassword {
		return ErrInvalidNewPassword
	}
	var encoded string
	if err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", id).Scan(&encoded); errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	} else if err != nil {
		return err
	}
	if !security.CheckPassword(encoded, currentPassword) {
		return ErrInvalidAdminPassword
	}
	encoded, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.setUserPassword(ctx, id, encoded)
}

func (s *Store) setUserPassword(ctx context.Context, id, encoded string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var username string
	if err := tx.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", id).Scan(&username); errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", encoded, time.Now().UTC().Unix(), id); err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	if username == "admin" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			adminPasswordKey, encoded, time.Now().UTC().Unix()); err != nil {
			return fmt.Errorf("update owner password compatibility record: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return tx.Commit()
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	if id == ownerUserID {
		return ErrOwnerImmutable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrUserNotFound
	}
	return tx.Commit()
}

func (s *Store) CreateUserSession(ctx context.Context, userID string, duration time.Duration) (string, time.Time, error) {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return "", time.Time{}, err
	}
	if !user.Enabled {
		return "", time.Time{}, ErrUserDisabled
	}
	return s.createSession(ctx, userID, duration)
}

func (s *Store) createSession(ctx context.Context, userID string, duration time.Duration) (string, time.Time, error) {
	token, err := security.RandomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(duration)
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)",
		security.TokenHash(token), userID, expiresAt.Unix(), now.Unix())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return token, expiresAt, nil
}

func (s *Store) SessionUser(ctx context.Context, token string) (User, bool, error) {
	if token == "" {
		return User{}, false, nil
	}
	var userID string
	var expiresAt int64
	err := s.db.QueryRowContext(ctx, "SELECT user_id, expires_at FROM sessions WHERE token_hash = ?", security.TokenHash(token)).Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("read session: %w", err)
	}
	if time.Now().UTC().Unix() >= expiresAt {
		return User{}, false, nil
	}
	if userID == "" {
		user, err := s.GetUser(ctx, ownerUserID)
		if errors.Is(err, ErrUserNotFound) {
			return User{ID: ownerUserID, Username: "admin", Role: RoleOwner, Enabled: true}, true, nil
		}
		return user, err == nil, err
	}
	user, err := s.GetUser(ctx, userID)
	if errors.Is(err, ErrUserNotFound) || (err == nil && !user.Enabled) {
		return User{}, false, nil
	}
	return user, err == nil, err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
