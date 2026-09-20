package engine

import (
	"context"
	"fmt"
	"log/slog"
)

// Called by ensure only after WhatsApp has invalidated this device. Keep the
// Wirely instance (token, settings, chats and webhooks) and replace only its
// revoked WhatsApp authentication session. The ensure lock prevents a new pair
// request from storing credentials before the old client's cleanup finishes.
func (m *Manager) discardLoggedOutSession(ctx context.Context, current *session) error {
	current.mu.Lock()
	current.retired = true
	current.reconnectAllowed = false
	current.connecting = false
	current.mu.Unlock()
	// Disconnect waits for the old WhatsApp handler queue before closing storage.
	current.client.Disconnect()
	current.cancel()

	// LoggedOut may be dispatched just before Whatsmeow deletes the device.
	// Finish deleting any revoked credential still present, never the instance DB.
	devices, err := current.container.GetAllDevices(ctx)
	if err != nil {
		return fmt.Errorf("read logged-out WhatsApp device: %w", err)
	}
	for _, device := range devices {
		if err := device.Delete(ctx); err != nil {
			return fmt.Errorf("clear revoked WhatsApp device: %w", err)
		}
	}
	err = current.container.Close()
	m.mu.Lock()
	if m.sessions[current.id] == current {
		delete(m.sessions, current.id)
	}
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("close logged-out WhatsApp session: %w", err)
	}
	slog.Info("Recreating WhatsApp session after logout", "instance_id", current.id)
	return nil
}
