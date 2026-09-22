package engine

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAdv"
	whatsmeowStore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func saveTestDevice(t *testing.T, current *session) {
	t.Helper()
	jid := types.NewJID("12345", types.DefaultUserServer)
	device := current.client.Store
	device.ID = &jid
	device.Account = &waAdv.ADVSignedDeviceIdentity{
		Details: []byte{1}, AccountSignature: make([]byte, 64),
		AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64),
	}
	if err := device.Save(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConnectPreparationReplacesDeletedDeviceAndPreservesInstance(t *testing.T) {
	for _, libraryDeletedFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "already deleted", false: "logout event before deletion"}[libraryDeletedFirst], func(t *testing.T) {
			ctx := context.Background()
			m, old := fallbackSession(t, "socks5://127.0.0.1:1080")
			saveTestDevice(t, old)
			token, err := m.store.GetInstanceToken(ctx, old.id)
			if err != nil {
				t.Fatal(err)
			}
			if libraryDeletedFirst {
				if err := old.client.Store.Delete(ctx); err != nil {
					t.Fatal(err)
				}
				if err := old.client.Connect(); !errors.Is(err, whatsmeowStore.ErrDeviceDeleted) {
					t.Fatalf("expected deleted-device failure, got %v", err)
				}
			}
			m.handleEvent(old, &events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
			// Reproduce the panel's close/disconnect after logout: the reset marker
			// must survive status changing from logged_out to disconnected.
			if err := m.Disconnect(old.id); err != nil {
				t.Fatal(err)
			}
			fresh, err := m.ensure(ctx, old.id)
			if err != nil {
				t.Fatal(err)
			}
			if fresh == old || fresh.client.Store.Deleted || fresh.client.Store.ID != nil {
				t.Fatal("must create a fresh unpaired device")
			}
			gotToken, err := m.store.GetInstanceToken(ctx, old.id)
			if err != nil || gotToken != token {
				t.Fatal("instance token changed")
			}
			proxy, err := m.store.GetInstanceProxy(ctx, old.id)
			if err != nil || proxy != "socks5://127.0.0.1:1080" {
				t.Fatal("instance proxy changed")
			}
			qrCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if _, err := fresh.client.GetQRChannel(qrCtx); err != nil {
				t.Fatalf("fresh device cannot pair: %v", err)
			}
			items := make(chan whatsmeow.QRChannelItem, 1)
			items <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "test-pairing-code", Timeout: time.Minute}
			close(items)
			m.consumeQR(fresh, items)
			png, err := m.QRCode(fresh.id)
			if err != nil || !bytes.HasPrefix(png, []byte{137, 80, 78, 71}) {
				t.Fatal("QR PNG unavailable")
			}
			// Late events from the discarded client must not overwrite the new QR.
			m.handleEvent(old, &events.Disconnected{})
			m.connectionError(old, whatsmeowStore.ErrDeviceDeleted)
			state, _ := m.State(fresh.id)
			stored, err := m.store.GetInstance(ctx, fresh.id)
			if err != nil || state.Status != "qr" || stored.Status != "qr" {
				t.Fatal("old session overwrote fresh pairing state")
			}
		})
	}
}

func TestConcurrentReconnectPreparationSharesFreshClient(t *testing.T) {
	m, old := fallbackSession(t, "")
	m.handleEvent(old, &events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
	var wg sync.WaitGroup
	got := make(chan *session, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			current, err := m.ensure(context.Background(), old.id)
			if err != nil {
				t.Error(err)
				return
			}
			got <- current
		}()
	}
	wg.Wait()
	close(got)
	var first *session
	for current := range got {
		if first == nil {
			first = current
		}
		if current == old || current != first {
			t.Fatal("concurrent requests created different clients")
		}
	}
}

func TestDeletedDeviceErrorMarksSessionForResetWithoutProxyFallback(t *testing.T) {
	m, old := fallbackSession(t, "socks5://127.0.0.1:1080")
	attempts := 0
	err := m.connectWithProxyFallback(old, func() error { attempts++; return whatsmeowStore.ErrDeviceDeleted })
	if attempts != 1 || old.proxyFallback {
		t.Fatal("deleted device is not a proxy failure")
	}
	m.connectionError(old, err)
	fresh, err := m.ensure(context.Background(), old.id)
	if err != nil || fresh == old {
		t.Fatal("deleted-device error must trigger recreation on next connect")
	}
}

func TestDeletedDeviceStopsBackgroundReconnectAndPreparesReset(t *testing.T) {
	m, old := fallbackSession(t, "socks5://127.0.0.1:1080")
	if old.client.AutoReconnectHook(whatsmeowStore.ErrDeviceDeleted) {
		t.Fatal("deleted device must stop the background reconnect loop")
	}
	old.mu.RLock()
	needsFreshDevice := old.needsFreshDevice
	proxyFallback := old.proxyFallback
	reconnectAllowed := old.reconnectAllowed
	old.mu.RUnlock()
	if !needsFreshDevice || proxyFallback || reconnectAllowed {
		t.Fatal("deleted device must be recreated without bypassing the proxy")
	}
	fresh, err := m.ensure(context.Background(), old.id)
	if err != nil || fresh == old {
		t.Fatal("next request must create a fresh WhatsApp device")
	}
}

func TestDisconnectWithoutLogoutKeepsExistingDevice(t *testing.T) {
	m, old := fallbackSession(t, "")
	if err := m.Disconnect(old.id); err != nil {
		t.Fatal(err)
	}
	current, err := m.ensure(context.Background(), old.id)
	if err != nil || current != old {
		t.Fatal("ordinary disconnect must preserve session")
	}
}
