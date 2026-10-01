package storage

import (
	"context"
	"strings"
	"testing"
)

func TestInstanceProxyIsEncryptedAndCanBeCleared(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	instance, err := store.CreateInstance(ctx, "Proxy")
	if err != nil {
		t.Fatal(err)
	}
	proxyURL := "socks5://wirely:secret@proxy.example:1080"
	if err := store.SaveInstanceProxy(ctx, instance.ID, proxyURL); err != nil {
		t.Fatal(err)
	}
	var ciphertext string
	if err := store.db.QueryRowContext(ctx, "SELECT proxy_ciphertext FROM instances WHERE id = ?", instance.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if ciphertext == "" || strings.Contains(ciphertext, "secret") || strings.Contains(ciphertext, "proxy.example") {
		t.Fatal("proxy credentials must only be persisted as ciphertext")
	}
	if got, err := store.GetInstanceProxy(ctx, instance.ID); err != nil || got != proxyURL {
		t.Fatalf("proxy was not decrypted: %q %v", got, err)
	}
	settings := ProxySettings{URL: proxyURL, AutoReconnect: true, RetryIntervalSeconds: 120, ReconnectAttempts: 8}
	if err := store.SaveInstanceProxySettings(ctx, instance.ID, settings); err != nil {
		t.Fatal(err)
	}
	saved, err := store.GetInstanceProxySettings(ctx, instance.ID)
	if err != nil || saved != settings {
		t.Fatalf("proxy recovery settings did not round trip: %#v %v", saved, err)
	}
	if err := store.SaveInstanceProxy(ctx, instance.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetInstanceProxy(ctx, instance.ID); err != nil || got != "" {
		t.Fatalf("proxy was not cleared: %q %v", got, err)
	}
	saved, err = store.GetInstanceProxySettings(ctx, instance.ID)
	if err != nil || !saved.AutoReconnect || saved.RetryIntervalSeconds != 120 || saved.ReconnectAttempts != 8 {
		t.Fatalf("changing only the address must preserve recovery settings: %#v %v", saved, err)
	}
}
