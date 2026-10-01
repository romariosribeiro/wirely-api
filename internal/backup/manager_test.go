package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCreateAndApplyRestore(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(dataDirectory, "whatsapp"), 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", filepath.Join(dataDirectory, "wirely.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE state (value TEXT); INSERT INTO state VALUES ('original')"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDirectory, "token.key"), []byte("original-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	manager, err := New(dataDirectory, 2, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	if created.Size == 0 || created.Reason != "manual" {
		t.Fatalf("unexpected backup metadata: %#v", created)
	}
	if err := os.WriteFile(filepath.Join(dataDirectory, "token.key"), []byte("changed-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PrepareRestore(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(dataDirectory)
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	restored, err := ApplyPending(dataDirectory)
	if err != nil || !restored {
		t.Fatalf("restore failed: restored=%v err=%v", restored, err)
	}
	key, err := os.ReadFile(filepath.Join(dataDirectory, "token.key"))
	if err != nil || string(key) != "original-key" {
		t.Fatalf("file was not restored: %q %v", key, err)
	}
	if _, err := manager.Path(created.ID); err != nil {
		t.Fatalf("selected backup was pruned while preparing restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDirectory, "backups", created.ID+".zip")); err != nil {
		t.Fatalf("backup archive was not preserved: %v", err)
	}
	if restoredAgain, err := ApplyPending(dataDirectory); err != nil || restoredAgain {
		t.Fatalf("restore marker was not cleared: restored=%v err=%v", restoredAgain, err)
	}
}

func TestBackupExcludesRuntimeCachesAndCleansStaleSnapshots(t *testing.T) {
	dataDirectory := filepath.Join(t.TempDir(), "data")
	write := func(relative, value string) {
		path := filepath.Join(dataDirectory, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", filepath.Join(dataDirectory, "wirely.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE state (value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	write("token.key", "key")
	write("whatsapp/session.dat", "session")
	write("whatsapp/received-media/cache/file.bin", "large cache")
	write("updates/wirely.ready", "staged binary")
	write("backups/.snapshot-interrupted/partial", "partial")

	manager, err := New(dataDirectory, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDirectory, "backups", ".snapshot-interrupted")); !os.IsNotExist(err) {
		t.Fatalf("stale snapshot was not removed: %v", err)
	}
	created, err := manager.Create(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(filepath.Join(dataDirectory, "backups", created.ID+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seen := map[string]bool{}
	for _, file := range reader.File {
		seen[file.Name] = true
	}
	if !seen["wirely.db"] || !seen["token.key"] || !seen["whatsapp/session.dat"] {
		t.Fatalf("essential files are missing from backup: %#v", seen)
	}
	for name := range seen {
		if strings.HasPrefix(name, "whatsapp/received-media/") || strings.HasPrefix(name, "updates/") || strings.HasPrefix(name, "backups/") {
			t.Fatalf("runtime file %q must not be backed up", name)
		}
	}
}

func TestCopyWithContextStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := copyWithContext(ctx, &bytes.Buffer{}, strings.NewReader("content")); err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestBackupPathRejectsTraversal(t *testing.T) {
	manager, err := New(t.TempDir(), 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../secret", "wirely-example.zip", "wirely/foo"} {
		if _, err := manager.Path(id); err != ErrNotFound {
			t.Fatalf("unsafe id %q was accepted: %v", id, err)
		}
	}
}
