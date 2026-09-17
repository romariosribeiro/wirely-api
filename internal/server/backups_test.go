package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/romariosribeiro/wirely-api/internal/backup"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func TestBackupPanelAPIIsOwnerOnly(t *testing.T) {
	directory := t.TempDir()
	store, err := storage.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ownerPassword, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := backup.New(directory, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Dependencies{Store: store, Backups: manager})
	ownerCookie := loginUser(t, app.Handler(), "admin", ownerPassword)

	created := requestWithCookie(app.Handler(), http.MethodPost, "/api/v1/backups", "", ownerCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("backup creation failed: %d %s", created.Code, created.Body.String())
	}
	var item backup.Info
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil || item.ID == "" {
		t.Fatalf("invalid backup response: %#v %v", item, err)
	}
	listed := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/backups", "", ownerCookie)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), item.ID) {
		t.Fatalf("backup was not listed: %d %s", listed.Code, listed.Body.String())
	}
	download := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/backups/"+item.ID+"/download", "", ownerCookie)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "application/zip" || download.Body.Len() == 0 {
		t.Fatalf("backup download failed: %d %#v", download.Code, download.Header())
	}

	admin, err := store.CreateUser(context.Background(), "backup.admin", "backup-admin-password", storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	adminCookie := loginUser(t, app.Handler(), admin.Username, "backup-admin-password")
	denied := requestWithCookie(app.Handler(), http.MethodGet, "/api/v1/backups", "", adminCookie)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("admin accessed owner backup: %d", denied.Code)
	}

	deleted := requestWithCookie(app.Handler(), http.MethodDelete, "/api/v1/backups/"+item.ID, "", ownerCookie)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("backup deletion failed: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := manager.Path(item.ID); err != backup.ErrNotFound {
		t.Fatalf("backup still exists at %s: %v", filepath.Join(directory, "backups"), err)
	}
}
