package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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
