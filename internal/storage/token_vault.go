package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The key is kept outside SQLite. Back up token.key together with wirely.db.
func (s *Store) openTokenVault(directory string) error {
	path := filepath.Join(directory, "token.key")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM instances WHERE api_token_ciphertext != ''").Scan(&count); err != nil {
			return fmt.Errorf("check encrypted tokens: %w", err)
		}
		if count > 0 {
			return errors.New("token.key is missing: restore it from backup; existing encrypted tokens must not be overwritten")
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		var file *os.File
		file, err = os.CreateTemp(directory, ".token-key-*")
		if err != nil {
			return fmt.Errorf("create token key: %w", err)
		}
		defer os.Remove(file.Name())
		defer file.Close()
		if _, err := file.Write(key); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		// Publish the complete key without ever replacing another process's key.
		if err := os.Link(file.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("read token key metadata: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return errors.New("token.key must be a regular file with permissions 0600")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read token key: %w", err)
	}
	if len(key) != 32 {
		return errors.New("invalid token.key: expected 32 bytes; restore the original key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	s.tokenCipher, err = cipher.NewGCM(block)
	return err
}

func (s *Store) encryptInstanceToken(id, token string) (string, error) {
	nonce := make([]byte, s.tokenCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.tokenCipher.Seal(nonce, nonce, []byte(token), []byte("wirely:instance-token:v1:"+id))
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

// GetInstanceToken is for authenticated administrators only, never list/auth responses.
// An empty token denotes a legacy hash-only record; reading must not rotate it.
func (s *Store) GetInstanceToken(ctx context.Context, id string) (string, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, "SELECT api_token_ciphertext FROM instances WHERE id = ?", id).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInstanceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read encrypted token: %w", err)
	}
	if encoded == "" {
		return "", nil
	}
	sealed, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil || len(sealed) < s.tokenCipher.NonceSize()+s.tokenCipher.Overhead() {
		return "", errors.New("invalid encrypted token")
	}
	n := s.tokenCipher.NonceSize()
	plain, err := s.tokenCipher.Open(nil, sealed[:n], sealed[n:], []byte("wirely:instance-token:v1:"+id))
	if err != nil {
		return "", errors.New("cannot decrypt token: check the original token.key")
	}
	return string(plain), nil
}
