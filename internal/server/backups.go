package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/backup"
)

type BackupManager interface {
	Status(context.Context) (backup.Status, error)
	Create(context.Context, string) (backup.Info, error)
	Path(string) (string, error)
	Delete(string) error
	PrepareRestore(context.Context, string) (backup.Info, error)
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup service is unavailable")
		return
	}
	status, err := s.backups.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup service is unavailable")
		return
	}
	item, err := s.backups.Create(r.Context(), "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backup")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup service is unavailable")
		return
	}
	id := r.PathValue("backupID")
	path, err := s.backups.Path(id)
	if errors.Is(err, backup.ErrNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read backup")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, id))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup service is unavailable")
		return
	}
	item, err := s.backups.PrepareRestore(r.Context(), r.PathValue("backupID"))
	if errors.Is(err, backup.ErrNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if errors.Is(err, backup.ErrInvalidArchive) {
		writeError(w, http.StatusUnprocessableEntity, "backup archive is invalid")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare restore")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"backup": item, "status": "restart_scheduled",
		"message": "Restore staged. Wirely will restart automatically.",
	})
	if s.restart != nil {
		go func() {
			time.Sleep(750 * time.Millisecond)
			s.restart()
		}()
	}
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup service is unavailable")
		return
	}
	err := s.backups.Delete(r.PathValue("backupID"))
	if errors.Is(err, backup.ErrNotFound) {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete backup")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
