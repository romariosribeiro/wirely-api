package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicGetUserAvatar(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.userInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Number  string `json:"number"`
		Preview bool   `json:"preview"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	avatar, err := s.whatsAppUsers.GetUserAvatar(r.Context(), instanceID, payload.Number, payload.Preview)
	if s.writeUserError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, avatar)
}

func (s *Server) publicSetContactBlocked(blocked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		instanceID, ok := s.userInstance(w, r)
		if !ok {
			return
		}
		var payload struct {
			Number string `json:"number"`
		}
		if err := decodeJSON(w, r, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
		users, err := s.whatsAppUsers.SetContactBlocked(r.Context(), instanceID, payload.Number, blocked)
		if s.writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": users})
	}
}

func (s *Server) publicGetBlocklist(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.userInstance(w, r)
	if !ok {
		return
	}
	users, err := s.whatsAppUsers.GetBlocklist(r.Context(), instanceID)
	if s.writeUserError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (s *Server) publicGetUserContacts(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.userInstance(w, r)
	if !ok {
		return
	}
	if s.contacts == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp contact store is unavailable")
		return
	}
	contacts, err := s.contacts.Contacts(r.Context(), instanceID)
	if s.writeUserError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": contacts})
}

func (s *Server) publicGetUsers(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.userInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Number []string `json:"number"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	users, err := s.whatsAppUsers.GetUsers(r.Context(), instanceID, payload.Number)
	if s.writeUserError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (s *Server) userInstance(w http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return "", false
	}
	if s.whatsAppUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp user service is unavailable")
		return "", false
	}
	return instanceID, true
}

func (s *Server) writeUserError(w http.ResponseWriter, err error) bool {
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
