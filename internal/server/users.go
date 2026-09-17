package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string       `json:"username"`
		Password string       `json:"password"`
		Role     storage.Role `json:"role"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	user, err := s.store.CreateUser(r.Context(), payload.Username, payload.Password, payload.Role)
	if userMutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("userID")
	current, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, storage.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	if current.Role == storage.RoleOwner {
		writeError(w, http.StatusConflict, storage.ErrOwnerImmutable.Error())
		return
	}
	var payload struct {
		Role    *storage.Role `json:"role"`
		Enabled *bool         `json:"enabled"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	role, enabled := current.Role, current.Enabled
	if payload.Role != nil {
		role = *payload.Role
	}
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	user, err := s.store.UpdateUser(r.Context(), id, role, enabled)
	if userMutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("userID")
	user, err := s.store.GetUser(r.Context(), id)
	if errors.Is(err, storage.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	if user.Role == storage.RoleOwner {
		writeError(w, http.StatusConflict, storage.ErrOwnerImmutable.Error())
		return
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if err := s.store.ResetUserPassword(r.Context(), id, payload.Password); userMutationError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteUser(r.Context(), r.PathValue("userID")); userMutationError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userMutationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, storage.ErrUserNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, storage.ErrUsernameUnavailable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, storage.ErrOwnerImmutable):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, storage.ErrInvalidUsername), errors.Is(err, storage.ErrInvalidUserRole), errors.Is(err, storage.ErrInvalidNewPassword):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed to update user")
	}
	return true
}
