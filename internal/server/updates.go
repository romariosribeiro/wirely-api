package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/updater"
)

type UpdateManager interface {
	Check(context.Context, bool) (updater.Status, error)
	Apply(context.Context) (updater.Status, error)
	Activate() error
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	if s.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "update service is unavailable")
		return
	}
	refresh := strings.EqualFold(r.URL.Query().Get("refresh"), "true") || r.URL.Query().Get("refresh") == "1"
	status, err := s.updates.Check(r.Context(), refresh)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	if s.updates == nil || s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "update or backup service is unavailable")
		return
	}
	status, err := s.updates.Check(r.Context(), true)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates")
		return
	}
	if !status.UpdateAvailable {
		writeError(w, http.StatusConflict, "Wirely is already up to date")
		return
	}
	if !status.CanApply {
		writeError(w, http.StatusConflict, "update package is unavailable for this server")
		return
	}
	backupInfo, err := s.backups.Create(r.Context(), "pre-update")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create the pre-update backup")
		return
	}
	status, err = s.updates.Apply(r.Context())
	if errors.Is(err, updater.ErrNoUpdate) || errors.Is(err, updater.ErrUpdateUnavailable) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply the verified update")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "restart_scheduled", "update": status, "backup": backupInfo,
		"message": "Update verified and installed. Wirely will restart automatically.",
	})
	go func() {
		time.Sleep(750 * time.Millisecond)
		_ = s.updates.Activate()
	}()
}
