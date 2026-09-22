package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.mau.fi/whatsmeow"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
)

// Only transport connection failures trigger fallback. Logout and rejected
// WhatsApp authentication events must never be bypassed by changing routes.
func (m *Manager) connectWithProxyFallback(current *session, connect func() error) error {
	err := connect()
	if err != nil && !errors.Is(err, whatsmeow.ErrAlreadyConnected) && !errors.Is(err, whatsmeowStore.ErrDeviceDeleted) && !errors.Is(err, context.Canceled) && m.activateProxyFallback(current) {
		return connect()
	}
	return err
}

func (m *Manager) handleReconnectFailure(current *session, err error) bool {
	current.mu.RLock()
	allowed := current.reconnectAllowed
	current.mu.RUnlock()
	if !allowed {
		return false
	}
	// A deleted device can never be recovered by changing network routes. Stop
	// whatsmeow's retry loop and let the next connect/pair request recreate only
	// the revoked WhatsApp device while preserving the Wirely instance.
	if errors.Is(err, whatsmeowStore.ErrDeviceDeleted) {
		m.connectionError(current, err)
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, whatsmeow.ErrAlreadyConnected) {
		return false
	}
	// Do not log the raw error: proxy errors may contain credentials or URLs.
	slog.Warn("WhatsApp reconnect attempt failed", "instance_id", current.id, "error_type", fmt.Sprintf("%T", err))
	m.activateProxyFallback(current)
	return true
}

func (m *Manager) activateProxyFallback(current *session) bool {
	current.mu.Lock()
	if !current.reconnectAllowed || !current.proxyConfigured || current.proxyFallback {
		current.mu.Unlock()
		return false
	}
	// This is called after a failed connection attempt, before the next retry.
	// Passing an empty address disables environment proxies too.
	if err := current.client.SetProxyAddress(""); err != nil {
		current.mu.Unlock()
		return false
	}
	current.proxyFallback = true
	current.mu.Unlock()
	slog.Warn("WhatsApp proxy unavailable; using direct VPS connection", "instance_id", current.id)
	m.setState(current, "connecting", "")
	return true
}
