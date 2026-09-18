package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) GetInstanceProxy(ctx context.Context, id string) (string, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, "SELECT proxy_ciphertext FROM instances WHERE id = ?", id).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInstanceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read instance proxy: %w", err)
	}
	return s.decryptInstanceSecret(id, "proxy", encoded)
}

func (s *Store) SaveInstanceProxy(ctx context.Context, id, proxyURL string) error {
	encoded := ""
	var err error
	if proxyURL != "" {
		encoded, err = s.encryptInstanceSecret(id, "proxy", proxyURL)
		if err != nil {
			return fmt.Errorf("encrypt instance proxy: %w", err)
		}
	}
	result, err := s.db.ExecContext(ctx, "UPDATE instances SET proxy_ciphertext = ?, updated_at = ? WHERE id = ?", encoded, time.Now().UTC().Unix(), id)
	if err != nil {
		return fmt.Errorf("save instance proxy: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrInstanceNotFound
	}
	return nil
}
