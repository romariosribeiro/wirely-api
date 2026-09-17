package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicGetProfile(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	profile, err := s.profile.GetProfile(r.Context(), instanceID)
	if s.writeProfileError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) publicUpdateProfile(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Name  *string `json:"name"`
		About *string `json:"about"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if s.writeProfileError(w, s.profile.UpdateProfile(r.Context(), instanceID, payload.Name, payload.About)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicSetProfilePhoto(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, engine.MaxProfilePhotoBytes+mediaRequestOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("photo must have at most %d MB", engine.MaxProfilePhotoBytes>>20))
		} else {
			writeError(w, http.StatusBadRequest, "request must use multipart/form-data")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "photo file is required")
		return
	}
	defer file.Close()
	photo, err := io.ReadAll(io.LimitReader(file, engine.MaxProfilePhotoBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read photo")
		return
	}
	if len(photo) > engine.MaxProfilePhotoBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("photo must have at most %d MB", engine.MaxProfilePhotoBytes>>20))
		return
	}
	if err := engine.ValidateProfilePhoto(photo); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	pictureID, err := s.profile.SetProfilePhoto(r.Context(), instanceID, photo)
	if s.writeProfileError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pictureId": pictureID})
}

func (s *Server) publicDeleteProfilePhoto(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	if _, err := s.profile.SetProfilePhoto(r.Context(), instanceID, nil); s.writeProfileError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicGetPrivacySettings(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	settings, err := s.profile.GetPrivacySettings(r.Context(), instanceID)
	if s.writeProfileError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) publicSetPrivacySetting(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.profileInstance(w, r)
	if !ok {
		return
	}
	var payload struct {
		Setting string `json:"setting"`
		Value   string `json:"value"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	settings, err := s.profile.SetPrivacySetting(r.Context(), instanceID, payload.Setting, payload.Value)
	if s.writeProfileError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) profileInstance(w http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return "", false
	}
	if s.profile == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return "", false
	}
	return instanceID, true
}

func (s *Server) writeProfileError(w http.ResponseWriter, err error) bool {
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
