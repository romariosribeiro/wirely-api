package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/security"
)

func TestTokenVaultPersistenceAndRotation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	instance, err := s.CreateInstance(ctx, "Persistent")
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.GetInstanceToken(ctx, instance.ID)
	if err != nil || token != instance.APIToken {
		t.Fatal("created token must be readable", err)
	}
	var encrypted, hash string
	if err := s.db.QueryRow("SELECT api_token_ciphertext, api_token_hash FROM instances WHERE id = ?", instance.ID).Scan(&encrypted, &hash); err != nil {
		t.Fatal(err)
	}
	if encrypted == "" || strings.Contains(encrypted, token) || hash != security.TokenHash(token) {
		t.Fatal("token must be encrypted with a separate authentication hash")
	}
	info, err := os.Stat(filepath.Join(dir, "token.key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("key permissions must be 0600", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.GetInstanceToken(ctx, instance.ID)
	if err != nil || reopened != token {
		t.Fatal("token must survive restart", err)
	}
	rotated, err := s.RotateInstanceToken(ctx, instance.ID)
	if err != nil || rotated == token {
		t.Fatal("rotation failed", err)
	}
	if got, err := s.GetInstanceToken(ctx, instance.ID); err != nil || got != rotated {
		t.Fatal("rotated token was not persisted", err)
	}
	if _, valid, err := s.AuthenticateInstanceToken(ctx, token); err != nil || valid {
		t.Fatal("old token must be revoked", err)
	}
	if _, valid, err := s.AuthenticateInstanceToken(ctx, rotated); err != nil || !valid {
		t.Fatal("new token must authenticate", err)
	}
	if _, err := s.GetInstanceToken(ctx, "missing"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatal("expected not found", err)
	}
	// A ciphertext copied to a different instance must not decrypt.
	other, err := s.CreateInstance(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE instances SET api_token_ciphertext = ? WHERE id = ?", encrypted, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetInstanceToken(ctx, other.ID); err == nil {
		t.Fatal("instance-bound encryption must reject swapped ciphertext")
	}
}

func TestTokenVaultLegacyAndMissingKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			_ = s.Close()
		}
	})
	instance, err := s.CreateInstance(ctx, "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE instances SET api_token_ciphertext = '' WHERE id = ?", instance.ID); err != nil {
		t.Fatal(err)
	}
	if token, err := s.GetInstanceToken(ctx, instance.ID); err != nil || token != "" {
		t.Fatal("legacy tokens must not be invented", err)
	}
	if _, valid, err := s.AuthenticateInstanceToken(ctx, instance.APIToken); err != nil || !valid {
		t.Fatal("reading must preserve legacy authentication", err)
	}
	if _, err := s.RotateInstanceToken(ctx, instance.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "token.key")
	if err := os.Rename(keyPath, keyPath+".backup"); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err == nil {
		t.Fatal("must not generate a replacement key over existing ciphertext")
	}
	if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing key was unexpectedly replaced")
	}
	if err := os.Rename(keyPath+".backup", keyPath); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if token, err := s.GetInstanceToken(ctx, instance.ID); err != nil || token == "" {
		t.Fatal("restoring key must restore access", err)
	}
}
