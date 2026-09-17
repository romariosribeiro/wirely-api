package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/backup"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/updater"
)

type fakeUpdateManager struct {
	status  updater.Status
	applied bool
}

func (manager *fakeUpdateManager) Check(context.Context, bool) (updater.Status, error) {
	return manager.status, nil
}

func (manager *fakeUpdateManager) Apply(context.Context) (updater.Status, error) {
	manager.applied = true
	return manager.status, nil
}

func (manager *fakeUpdateManager) Activate() error { return nil }

func TestUpdateAPIRequiresOwnerAndCreatesBackup(t *testing.T) {
	directory := t.TempDir()
	store, err := storage.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := store.EnsureAdmin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	backups, err := backup.New(directory, 7, 0)
	if err != nil {
		t.Fatal(err)
	}
	updates := &fakeUpdateManager{status: updater.Status{
		CurrentVersion: "0.9.0", LatestVersion: "1.0.0", UpdateAvailable: true, CanApply: true, CheckedAt: time.Now().UTC(),
	}}
	app := New(Dependencies{Store: store, Backups: backups, Updates: updates})
	ownerCookie := loginUser(t, app.Handler(), "admin", password)

	checked := requestWithCookie(app.Handler(), http.MethodGet, "/api/system/update", "", ownerCookie)
	if checked.Code != http.StatusOK {
		t.Fatalf("update check failed: %d %s", checked.Code, checked.Body.String())
	}
	applied := requestWithCookie(app.Handler(), http.MethodPost, "/api/system/update", "", ownerCookie)
	if applied.Code != http.StatusAccepted || !updates.applied {
		t.Fatalf("update apply failed: %d %s", applied.Code, applied.Body.String())
	}
	items, err := backups.List(context.Background())
	if err != nil || len(items) != 1 || items[0].Reason != "pre-update" {
		t.Fatalf("pre-update backup was not created: %#v %v", items, err)
	}

	admin, err := store.CreateUser(context.Background(), "updates.admin", "updates-admin-password", storage.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	adminCookie := loginUser(t, app.Handler(), admin.Username, "updates-admin-password")
	denied := requestWithCookie(app.Handler(), http.MethodPost, "/api/system/update", "", adminCookie)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("admin applied update: %d", denied.Code)
	}
}
