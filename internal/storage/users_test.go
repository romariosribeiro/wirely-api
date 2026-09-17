package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/security"
)

func TestOwnerMigrationPreservesLegacySession(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	legacySession, _, err := store.CreateSession(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	password, err := store.EnsureAdmin(ctx)
	if err != nil || password == "" {
		t.Fatalf("owner bootstrap failed: password=%q err=%v", password, err)
	}
	if second, err := store.EnsureAdmin(ctx); err != nil || second != "" {
		t.Fatalf("owner bootstrap repeated: password=%q err=%v", second, err)
	}
	owner, valid, err := store.SessionUser(ctx, legacySession)
	if err != nil || !valid || owner.Username != "admin" || owner.Role != RoleOwner {
		t.Fatalf("legacy session was not migrated: %#v valid=%v err=%v", owner, valid, err)
	}
	authenticated, valid, err := store.AuthenticateUser(ctx, "ADMIN", password)
	if err != nil || !valid || authenticated.ID != ownerUserID {
		t.Fatalf("owner authentication failed: %#v valid=%v err=%v", authenticated, valid, err)
	}
}

func TestUserLifecycleRevokesSessions(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	const initialPassword = "operator-password-123"
	user, err := store.CreateUser(ctx, "Support.Team", initialPassword, RoleOperator)
	if err != nil || user.Username != "support.team" || user.Role != RoleOperator || !user.Enabled {
		t.Fatalf("create user failed: %#v %v", user, err)
	}
	if _, err := store.CreateUser(ctx, "SUPPORT.TEAM", "another-password-123", RoleViewer); !errors.Is(err, ErrUsernameUnavailable) {
		t.Fatalf("duplicate username accepted: %v", err)
	}
	session, _, err := store.CreateUserSession(ctx, user.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if principal, valid, err := store.SessionUser(ctx, session); err != nil || !valid || principal.ID != user.ID {
		t.Fatalf("user session invalid: %#v %v %v", principal, valid, err)
	}
	updated, err := store.UpdateUser(ctx, user.ID, RoleViewer, true)
	if err != nil || updated.Role != RoleViewer {
		t.Fatalf("role update failed: %#v %v", updated, err)
	}
	if _, valid, err := store.SessionUser(ctx, session); err != nil || valid {
		t.Fatalf("role update did not revoke session: valid=%v err=%v", valid, err)
	}
	const resetPassword = "reset-password-456"
	if err := store.ResetUserPassword(ctx, user.ID, resetPassword); err != nil {
		t.Fatal(err)
	}
	if _, valid, _ := store.AuthenticateUser(ctx, user.Username, initialPassword); valid {
		t.Fatal("old password remained valid after reset")
	}
	if _, valid, err := store.AuthenticateUser(ctx, user.Username, resetPassword); err != nil || !valid {
		t.Fatalf("reset password does not authenticate: valid=%v err=%v", valid, err)
	}
	if _, err := store.UpdateUser(ctx, ownerUserID, RoleAdmin, false); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("owner mutation was accepted: %v", err)
	}
	if err := store.DeleteUser(ctx, ownerUserID); !errors.Is(err, ErrOwnerImmutable) {
		t.Fatalf("owner deletion was accepted: %v", err)
	}
	if err := store.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUser(ctx, user.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("deleted user remains: %v", err)
	}
}

func TestRoleHierarchy(t *testing.T) {
	if !RoleAllows(RoleOwner, RoleOwner) || !RoleAllows(RoleAdmin, RoleOperator) || !RoleAllows(RoleOperator, RoleViewer) {
		t.Fatal("valid role inheritance rejected")
	}
	if RoleAllows(RoleViewer, RoleOperator) || RoleAllows(RoleOperator, RoleAdmin) || RoleAllows(Role("invalid"), RoleViewer) {
		t.Fatal("invalid role escalation accepted")
	}
}

func TestUsersMigrationFromVersionFiveSchema(t *testing.T) {
	directory := t.TempDir()
	database, err := sql.Open("sqlite", filepath.Join(directory, "wirely.db"))
	if err != nil {
		t.Fatal(err)
	}
	const password = "legacy-owner-password-123"
	encoded, err := security.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	legacyToken := "legacy-session-token"
	if _, err = database.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL);
CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, expires_at INTEGER NOT NULL, created_at INTEGER NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec("INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)", adminPasswordKey, encoded, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec("INSERT INTO sessions (token_hash, expires_at, created_at) VALUES (?, ?, ?)",
		security.TokenHash(legacyToken), time.Now().Add(time.Hour).Unix(), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if generated, err := store.EnsureAdmin(ctx); err != nil || generated != "" {
		t.Fatalf("legacy owner was replaced: generated=%q err=%v", generated, err)
	}
	owner, valid, err := store.AuthenticateUser(ctx, "admin", password)
	if err != nil || !valid || owner.Role != RoleOwner {
		t.Fatalf("legacy password was not preserved: %#v valid=%v err=%v", owner, valid, err)
	}
	principal, valid, err := store.SessionUser(ctx, legacyToken)
	if err != nil || !valid || principal.ID != owner.ID {
		t.Fatalf("legacy session was not preserved: %#v valid=%v err=%v", principal, valid, err)
	}

	const rollbackPassword = "password-changed-during-rollback-456"
	rollbackHash, err := security.HashPassword(rollbackPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE settings SET value = ? WHERE key = ?", rollbackHash, adminPasswordKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if _, valid, err := store.AuthenticateUser(ctx, "admin", rollbackPassword); err != nil || !valid {
		t.Fatalf("rollback password was not synchronized: valid=%v err=%v", valid, err)
	}
}
