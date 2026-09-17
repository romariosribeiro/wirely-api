package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicDownloadReceivedMedia(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.receivedMedia == nil {
		writeError(w, http.StatusServiceUnavailable, "received media storage is unavailable")
		return
	}
	media, err := s.receivedMedia.OpenReceivedMedia(r.Context(), instanceID, r.PathValue("messageID"))
	if errors.Is(err, engine.ErrReceivedMediaNotFound) {
		writeError(w, http.StatusNotFound, "received media not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open received media")
		return
	}
	defer media.File.Close()
	switch r.URL.Query().Get("format") {
	case "", "file":
		w.Header().Set("Content-Type", media.MIMEType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": media.FileName}))
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, media.FileName, media.SavedAt, media.File)
	case "base64":
		writeReceivedMediaBase64(w, r.PathValue("messageID"), media)
	default:
		writeError(w, http.StatusUnprocessableEntity, "format must be file or base64")
	}
}

func writeReceivedMediaBase64(w http.ResponseWriter, messageID string, media engine.ReceivedMedia) {
	quotedID, _ := json.Marshal(messageID)
	quotedType, _ := json.Marshal(media.Kind)
	quotedMIME, _ := json.Marshal(media.MIMEType)
	quotedName, _ := json.Marshal(media.FileName)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{\"messageId\":" + string(quotedID) + ",\"type\":" + string(quotedType) +
		",\"mimetype\":" + string(quotedMIME) + ",\"fileName\":" + string(quotedName) +
		",\"size\":" + strconv.FormatInt(media.Size, 10) + ",\"data\":\""))
	encoder := base64.NewEncoder(base64.StdEncoding, w)
	_, copyErr := io.Copy(encoder, media.File)
	closeErr := encoder.Close()
	if copyErr == nil && closeErr == nil {
		_, _ = w.Write([]byte("\"}"))
	}
}
