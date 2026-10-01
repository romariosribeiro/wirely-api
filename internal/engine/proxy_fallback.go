package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"go.mau.fi/whatsmeow"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
	"golang.org/x/net/proxy"

	"github.com/romariosribeiro/wirely-api/internal/storage"
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
	m.startProxyRecovery(current)
	return true
}

func (m *Manager) startProxyRecovery(current *session) {
	current.mu.Lock()
	current.proxyRecoveryGen++
	generation := current.proxyRecoveryGen
	settings := current.proxySettings
	allowed := current.reconnectAllowed && current.proxyConfigured && current.proxyFallback && settings.AutoReconnect
	current.mu.Unlock()
	if allowed {
		go m.recoverProxy(current, generation, settings)
	}
}

func (m *Manager) recoverProxy(current *session, generation uint64, settings storage.ProxySettings) {
	attempt := 0
	for settings.ReconnectAttempts == 0 || attempt < settings.ReconnectAttempts {
		timer := time.NewTimer(m.proxyDelay(settings.RetryIntervalSeconds))
		select {
		case <-current.client.BackgroundEventCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
		current.mu.RLock()
		active := current.proxyRecoveryGen == generation && current.reconnectAllowed && current.proxyFallback && current.proxySettings.AutoReconnect
		current.mu.RUnlock()
		if !active {
			return
		}
		attempt++
		probeContext, cancel := context.WithTimeout(current.client.BackgroundEventCtx, 15*time.Second)
		err := m.proxyProbe(probeContext, settings.URL)
		cancel()
		if err != nil {
			slog.Info("WhatsApp proxy recovery check failed", "instance_id", current.id, "attempt", attempt)
			continue
		}
		m.proxyReturn(current)
		return
	}
	slog.Warn("WhatsApp proxy recovery attempts exhausted", "instance_id", current.id, "attempts", attempt)
}

func (m *Manager) returnToProxy(current *session) {
	current.mu.Lock()
	if !current.reconnectAllowed || !current.proxyConfigured || !current.proxyFallback || !current.proxySettings.AutoReconnect {
		current.mu.Unlock()
		return
	}
	address := current.proxySettings.URL
	if err := current.client.SetProxyAddress(address); err != nil {
		current.mu.Unlock()
		return
	}
	current.proxyFallback = false
	current.proxyRecoveryGen++
	current.mu.Unlock()
	slog.Info("WhatsApp proxy is available again; reconnecting through proxy", "instance_id", current.id)
	current.client.Disconnect()
	current.mu.Lock()
	current.connecting = false
	current.mu.Unlock()
	_ = m.Connect(current.id)
}

func probeProxy(ctx context.Context, address string) error {
	parsed, err := url.Parse(address)
	if err != nil {
		return err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if parsed.Scheme == "socks5" {
		proxyDialer, err := proxy.FromURL(parsed, dialer)
		if err != nil {
			return err
		}
		contextDialer, ok := proxyDialer.(proxy.ContextDialer)
		if !ok {
			return errors.New("SOCKS5 proxy does not support context dialing")
		}
		connection, err := contextDialer.DialContext(ctx, "tcp", "web.whatsapp.com:443")
		if err != nil {
			return err
		}
		return connection.Close()
	}
	transport := &http.Transport{Proxy: http.ProxyURL(parsed), DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://web.whatsapp.com/", nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return err
	}
	return response.Body.Close()
}
