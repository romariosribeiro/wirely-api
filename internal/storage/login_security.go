package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
)

const (
	LoginMaxFailures = 5
	LoginWindow      = 15 * time.Minute
	LoginLockout     = 15 * time.Minute
)

// AuthenticateUserProtected authenticates a panel user and persistently tracks
// failures per normalized username and source IP. A successful login clears the
// corresponding failure record.
func (s *Store) AuthenticateUserProtected(ctx context.Context, username, password, sourceIP string) (User, bool, time.Time, error) {
	lookupUsername, err := normalizeUsername(username)
	trackingUsername := lookupUsername
	if err != nil {
		lookupUsername = ""
		trackingUsername = "invalid-" + security.TokenHash(strings.ToLower(strings.TrimSpace(username)))[:20]
	}
	sourceIP = strings.TrimSpace(sourceIP)
	if sourceIP == "" {
		sourceIP = "unknown"
	}
	if len(sourceIP) > 64 {
		sourceIP = sourceIP[:64]
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, false, time.Time{}, fmt.Errorf("start protected login: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	var failures int
	var windowStartedAt, lockedUntil int64
	err = tx.QueryRowContext(ctx, `SELECT failures, window_started_at, locked_until
FROM login_attempts WHERE username = ? AND source_ip = ?`, trackingUsername, sourceIP).
		Scan(&failures, &windowStartedAt, &lockedUntil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, false, time.Time{}, fmt.Errorf("read login attempts: %w", err)
	}
	if lockedUntil > now.Unix() {
		return User{}, false, time.Unix(lockedUntil, 0).UTC(), nil
	}
	if errors.Is(err, sql.ErrNoRows) || windowStartedAt <= now.Add(-LoginWindow).Unix() {
		failures = 0
		windowStartedAt = now.Unix()
	}

	var user User
	var encoded string
	var enabled int
	var createdAt, updatedAt, lastLoginAt int64
	if lookupUsername != "" {
		err = tx.QueryRowContext(ctx, `SELECT id, username, password_hash, role, enabled, created_at, updated_at, last_login_at
FROM users WHERE username = ?`, lookupUsername).
			Scan(&user.ID, &user.Username, &encoded, &user.Role, &enabled, &createdAt, &updatedAt, &lastLoginAt)
	} else {
		err = sql.ErrNoRows
	}
	valid := err == nil && enabled != 0 && security.CheckPassword(encoded, password)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, false, time.Time{}, fmt.Errorf("read protected user credentials: %w", err)
	}

	if valid {
		if _, err := tx.ExecContext(ctx, "DELETE FROM login_attempts WHERE username = ? AND source_ip = ?", trackingUsername, sourceIP); err != nil {
			return User{}, false, time.Time{}, fmt.Errorf("clear login attempts: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET last_login_at = ? WHERE id = ?", now.Unix(), user.ID); err != nil {
			return User{}, false, time.Time{}, fmt.Errorf("record user login: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return User{}, false, time.Time{}, fmt.Errorf("commit protected login: %w", err)
		}
		user.Enabled = true
		user.CreatedAt = time.Unix(createdAt, 0).UTC()
		user.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		user.LastLoginAt = &now
		return user, true, time.Time{}, nil
	}

	failures++
	lockedAt := int64(0)
	if failures >= LoginMaxFailures {
		lockedAt = now.Add(LoginLockout).Unix()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO login_attempts
(username, source_ip, failures, window_started_at, locked_until, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(username, source_ip) DO UPDATE SET
failures = excluded.failures,
window_started_at = excluded.window_started_at,
locked_until = excluded.locked_until,
updated_at = excluded.updated_at`, trackingUsername, sourceIP, failures, windowStartedAt, lockedAt, now.Unix())
	if err != nil {
		return User{}, false, time.Time{}, fmt.Errorf("record login failure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return User{}, false, time.Time{}, fmt.Errorf("commit login failure: %w", err)
	}
	if lockedAt > 0 {
		return User{}, false, time.Unix(lockedAt, 0).UTC(), nil
	}
	return User{}, false, time.Time{}, nil
}

func (s *Store) PruneLoginAttempts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM login_attempts WHERE updated_at < ?", time.Now().UTC().Add(-24*time.Hour).Unix())
	if err != nil {
		return fmt.Errorf("prune login attempts: %w", err)
	}
	return nil
}
