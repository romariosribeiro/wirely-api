package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

const mediaRequestOverhead = 1 << 20

func (s *Server) publicSendMediaMessage(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.sender == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	media, recipient, status, err := readMediaRequest(w, r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	options, err := parseMultipartSendOptions(r.FormValue("options"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stopPresence, ok := s.beginSendOptions(w, r, instanceID, recipient, options)
	if !ok {
		return
	}
	defer stopPresence()
	result, err := s.sender.SendMedia(r.Context(), instanceID, recipient, media)
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func readMediaRequest(w http.ResponseWriter, r *http.Request) (engine.MediaPayload, string, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, engine.MaxMediaBytes+mediaRequestOverhead)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maximum *http.MaxBytesError
		if errors.As(err, &maximum) || strings.Contains(err.Error(), "request body too large") {
			return engine.MediaPayload{}, "", http.StatusRequestEntityTooLarge, fmt.Errorf("file must have at most %d MB", engine.MaxMediaBytes>>20)
		}
		return engine.MediaPayload{}, "", http.StatusBadRequest, errors.New("request must use multipart/form-data")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	kind := engine.MediaKind(strings.ToLower(strings.TrimSpace(r.FormValue("type"))))
	switch kind {
	case engine.MediaImage, engine.MediaVideo, engine.MediaAudio, engine.MediaDocument, engine.MediaSticker:
	default:
		return engine.MediaPayload{}, "", http.StatusUnprocessableEntity, errors.New("type must be image, video, audio, document, or sticker")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return engine.MediaPayload{}, "", http.StatusBadRequest, errors.New("file is required")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, engine.MaxMediaBytes+1))
	if err != nil {
		return engine.MediaPayload{}, "", http.StatusBadRequest, errors.New("failed to read file")
	}
	if len(data) > engine.MaxMediaBytes {
		return engine.MediaPayload{}, "", http.StatusRequestEntityTooLarge, fmt.Errorf("file must have at most %d MB", engine.MaxMediaBytes>>20)
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	detected := http.DetectContentType(data)
	if mimeType == "" || mimeType == "application/octet-stream" || kind == engine.MediaImage || kind == engine.MediaSticker {
		mimeType = detected
	}
	voice, _ := strconv.ParseBool(r.FormValue("voice"))
	media := engine.MediaPayload{Kind: kind, Data: data, MIMEType: mimeType, FileName: header.Filename, Caption: r.FormValue("caption"), Voice: voice}
	if err := engine.ValidateMedia(&media); err != nil {
		return engine.MediaPayload{}, "", http.StatusUnprocessableEntity, err
	}
	return media, r.FormValue("recipient"), http.StatusUnprocessableEntity, nil
}
