package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicSetPresence(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Presence string `json:"presence"`
	}
	s.handlePresence(w, r, &payload, func(instanceID string) (engine.PresenceState, error) {
		return s.presenceState.SetPresence(r.Context(), instanceID, payload.Presence)
	})
}

func (s *Server) publicSubscribePresence(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Phone string `json:"phone"`
	}
	s.handlePresence(w, r, &payload, func(instanceID string) (engine.PresenceState, error) {
		return s.presenceState.SubscribePresence(r.Context(), instanceID, payload.Phone)
	})
}

func (s *Server) publicGetPresence(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.presenceState == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp presence engine is unavailable")
		return
	}
	phone := strings.TrimSpace(r.PathValue("phone"))
	result, err := s.presenceState.GetPresence(r.Context(), instanceID, phone)
	if writePresenceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request, payload any, action func(string) (engine.PresenceState, error)) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.presenceState == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp presence engine is unavailable")
		return
	}
	if err := decodeJSON(w, r, payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := action(instanceID)
	if writePresenceError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writePresenceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
	} else {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
	return true
}
