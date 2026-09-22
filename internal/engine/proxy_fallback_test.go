package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/storage"
	"go.mau.fi/whatsmeow"
)

func fallbackSession(t *testing.T, proxy string) (*Manager, *session) {
	t.Helper()
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	instance, err := s.CreateInstance(context.Background(), "fallback-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveInstanceProxy(context.Background(), instance.ID, proxy); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(dir, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	current, err := m.ensure(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.reconnectAllowed = true
	return m, current
}

func TestProxyFailureRetriesDirectAndPreservesConfiguration(t *testing.T) {
	const proxy = "socks5://127.0.0.1:1080"
	m, current := fallbackSession(t, proxy)
	attempts := 0
	err := m.connectWithProxyFallback(current, func() error {
		attempts++
		state, err := m.State(current.id)
		if err != nil {
			t.Fatal(err)
		}
		if attempts == 1 {
			if state.ConnectionRoute != "proxy" {
				t.Fatal("first attempt must use proxy")
			}
			return errors.New("proxy connection refused")
		}
		if state.ConnectionRoute != "direct" || !state.ProxyFallback {
			t.Fatal("retry must use direct route")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
	saved, err := m.store.GetInstanceProxy(context.Background(), current.id)
	if err != nil || saved != proxy {
		t.Fatal("fallback must preserve configured proxy")
	}
	if m.activateProxyFallback(current) {
		t.Fatal("must not switch routes repeatedly")
	}
	if err := m.SetProxy(current.id, proxy); err != nil {
		t.Fatal(err)
	}
	state, _ := m.State(current.id)
	if state.ProxyFallback || state.ConnectionRoute != "proxy" {
		t.Fatal("reapplying proxy must restore proxy route")
	}
}

func TestProxyFallbackDoesNotRetrySuccessfulOrDirectConnections(t *testing.T) {
	for _, tc := range []struct {
		name, proxy string
		failure     bool
	}{
		{"healthy proxy", "http://127.0.0.1:1080", false},
		{"direct failure", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, current := fallbackSession(t, tc.proxy)
			attempts := 0
			failure := errors.New("connection refused")
			err := m.connectWithProxyFallback(current, func() error {
				attempts++
				if tc.failure {
					return failure
				}
				return nil
			})
			if attempts != 1 || current.proxyFallback {
				t.Fatal("unexpected fallback")
			}
			if tc.failure && !errors.Is(err, failure) {
				t.Fatal("direct error must be preserved")
			}
		})
	}
}

func TestProxyFallbackContinuesRetriesButHonorsManualDisconnect(t *testing.T) {
	m, current := fallbackSession(t, "socks5://127.0.0.1:1080")
	if !current.client.AutoReconnectHook(errors.New("proxy unavailable")) || !current.proxyFallback {
		t.Fatal("failed background reconnect must switch to direct and continue retrying")
	}
	if !current.client.AutoReconnectHook(errors.New("direct unavailable")) {
		t.Fatal("direct retry must continue")
	}
	if err := m.Disconnect(current.id); err != nil {
		t.Fatal(err)
	}
	if current.client.AutoReconnectHook(errors.New("late failure")) {
		t.Fatal("manual disconnect must stop retries")
	}
	state, _ := m.State(current.id)
	if state.Status != "disconnected" {
		t.Fatal("manual disconnect must remain disconnected")
	}
}

func TestProxyFallbackReturnsDirectFailure(t *testing.T) {
	m, current := fallbackSession(t, "http://127.0.0.1:1080")
	failure := errors.New("direct unavailable")
	attempts := 0
	err := m.connectWithProxyFallback(current, func() error {
		attempts++
		if attempts == 1 {
			return errors.New("proxy unavailable")
		}
		return failure
	})
	if attempts != 2 || !errors.Is(err, failure) {
		t.Fatal("must return failed direct retry without looping")
	}
}

func TestRejectedHTTPProxyTriggersFallback(t *testing.T) {
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	m, current := fallbackSession(t, proxy.URL)
	attempts := 0
	err := m.connectWithProxyFallback(current, func() error {
		attempts++
		if attempts == 1 {
			return current.client.Connect()
		}
		// Avoid contacting WhatsApp from a unit test after changing route.
		return nil
	})
	if err != nil || attempts != 2 || requests.Load() != 1 || !current.proxyFallback {
		t.Fatalf("HTTP proxy failure did not fall back: attempts=%d requests=%d error=%v", attempts, requests.Load(), err)
	}
}

func TestCanceledOrAlreadyConnectedStopsReconnectWithoutChangingProxy(t *testing.T) {
	for _, failure := range []error{context.Canceled, whatsmeow.ErrAlreadyConnected} {
		m, current := fallbackSession(t, "socks5://127.0.0.1:1080")
		if current.client.AutoReconnectHook(failure) || current.proxyFallback {
			t.Fatal("terminal reconnect error must stop without changing proxy")
		}
		attempts := 0
		err := m.connectWithProxyFallback(current, func() error { attempts++; return failure })
		if attempts != 1 || !errors.Is(err, failure) {
			t.Fatal("non-transport error must not change proxy")
		}
	}
}

func TestStateDoesNotReportConnectedWithoutLiveSocket(t *testing.T) {
	m, current := fallbackSession(t, "")
	m.setState(current, "connected", "")
	state, err := m.State(current.id)
	if err != nil || state.Status != "connecting" {
		t.Fatal("cached connected status must not hide a missing socket")
	}
}
