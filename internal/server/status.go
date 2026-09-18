package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicSendStatusText(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.statuses == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp status engine is unavailable")
		return
	}
	var payload engine.StatusTextPayload
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.statuses.SendStatusText(r.Context(), instanceID, payload)
	if writeStatusError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) publicSendStatusMedia(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.statuses == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp status engine is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, engine.MaxMediaBytes+mediaRequestOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file must have at most %d MB", engine.MaxMediaBytes>>20))
		} else {
			writeError(w, http.StatusBadRequest, "request must use multipart/form-data")
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, engine.MaxMediaBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	payload := engine.MediaPayload{Kind: engine.MediaKind(strings.ToLower(strings.TrimSpace(r.FormValue("type")))), Data: data, MIMEType: mimeType, FileName: header.Filename, Caption: r.FormValue("caption")}
	result, err := s.statuses.SendStatusMedia(r.Context(), instanceID, payload)
	if writeStatusError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func writeStatusError(w http.ResponseWriter, err error) bool {
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
