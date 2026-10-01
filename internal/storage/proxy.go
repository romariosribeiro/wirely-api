package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const DefaultProxyRetryInterval = 60

type ProxySettings struct {
	URL                  string `json:"-"`
	AutoReconnect        bool   `json:"autoReconnect"`
	RetryIntervalSeconds int    `json:"retryIntervalSeconds"`
	ReconnectAttempts    int    `json:"reconnectAttempts"`
}

func normalizeProxySettings(settings ProxySettings) ProxySettings {
	if settings.RetryIntervalSeconds < DefaultProxyRetryInterval {
		settings.RetryIntervalSeconds = DefaultProxyRetryInterval
	}
	if settings.ReconnectAttempts < 0 {
		settings.ReconnectAttempts = 0
	}
	return settings
}

func (s *Store) GetInstanceProxySettings(ctx context.Context, id string) (ProxySettings, error) {
	var encoded string
	var settings ProxySettings
	err := s.db.QueryRowContext(ctx, `SELECT proxy_ciphertext, proxy_auto_reconnect, proxy_retry_interval, proxy_retry_attempts
FROM instances WHERE id = ?`, id).Scan(&encoded, &settings.AutoReconnect, &settings.RetryIntervalSeconds, &settings.ReconnectAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, ErrInstanceNotFound
	}
	if err != nil {
		return settings, fmt.Errorf("read instance proxy settings: %w", err)
	}
	settings = normalizeProxySettings(settings)
	if encoded == "" {
		return settings, nil
	}
	settings.URL, err = s.decryptInstanceSecret(id, "proxy", encoded)
	if err != nil {
		return ProxySettings{}, err
	}
	return settings, nil
}

func (s *Store) GetInstanceProxy(ctx context.Context, id string) (string, error) {
	settings, err := s.GetInstanceProxySettings(ctx, id)
	return settings.URL, err
}

func (s *Store) SaveInstanceProxy(ctx context.Context, id, proxyURL string) error {
	settings, err := s.GetInstanceProxySettings(ctx, id)
	if err != nil {
		return err
	}
	settings.URL = proxyURL
	return s.SaveInstanceProxySettings(ctx, id, settings)
}

func (s *Store) SaveInstanceProxySettings(ctx context.Context, id string, settings ProxySettings) error {
	settings = normalizeProxySettings(settings)
	encoded := ""
	var err error
	if settings.URL != "" {
		encoded, err = s.encryptInstanceSecret(id, "proxy", settings.URL)
		if err != nil {
			return fmt.Errorf("encrypt instance proxy: %w", err)
		}
	}
	result, err := s.db.ExecContext(ctx, `UPDATE instances
SET proxy_ciphertext = ?, proxy_auto_reconnect = ?, proxy_retry_interval = ?, proxy_retry_attempts = ?, updated_at = ?
WHERE id = ?`, encoded, settings.AutoReconnect, settings.RetryIntervalSeconds, settings.ReconnectAttempts, time.Now().UTC().Unix(), id)
	if err != nil {
		return fmt.Errorf("save instance proxy settings: %w", err)
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
