package server

import (
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func (s *Server) getAlertIntegrations(w http.ResponseWriter, r *http.Request) {
	config, err := s.store.GetAlertIntegrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load alert integrations")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) updateAlertIntegrations(w http.ResponseWriter, r *http.Request) {
	var payload storage.AlertIntegrationsUpdate
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	config, err := s.store.SaveAlertIntegrations(r.Context(), payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func (s *Server) testTelegramAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alerts == nil {
		writeError(w, http.StatusServiceUnavailable, "alert delivery is unavailable")
		return
	}
	if err := s.alerts.TestTelegram(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *Server) testSMTPAlerts(w http.ResponseWriter, r *http.Request) {
	if s.alerts == nil {
		writeError(w, http.StatusServiceUnavailable, "alert delivery is unavailable")
		return
	}
	if err := s.alerts.TestSMTP(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
