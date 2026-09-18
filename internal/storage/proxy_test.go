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
	if err := store.SaveInstanceProxy(ctx, instance.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetInstanceProxy(ctx, instance.ID); err != nil || got != "" {
		t.Fatalf("proxy was not cleared: %q %v", got, err)
	}
}
