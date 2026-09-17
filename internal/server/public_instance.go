package server

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/engine"
)

func (s *Server) publicGetInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	instance, err := s.store.GetInstance(r.Context(), instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "instance not found")
		return
	}
	writeJSON(w, http.StatusOK, instance)
}

func (s *Server) publicConnectInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Connect(instanceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "connecting"})
}

func (s *Server) publicDisconnectInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Disconnect(instanceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicLogoutInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.Logout(r.Context(), instanceID); err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, engine.ErrNotConnected) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicPairInstance(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Phone string `json:"phone"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	code, err := s.engine.PairPhone(r.Context(), instanceID, payload.Phone)
	if errors.Is(err, engine.ErrPairingUnavailable) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expiresIn": 160})
}

func (s *Server) publicDeleteInstanceProxy(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	if err := s.engine.ClearProxy(instanceID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicInstanceQR(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	png, err := s.engine.QRCode(instanceID)
	if errors.Is(err, engine.ErrQRUnavailable) {
		writeError(w, http.StatusNotFound, "QR code is not available")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render QR code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Query().Get("format") == "base64" {
		writeJSON(w, http.StatusOK, map[string]string{"mimetype": "image/png", "data": base64.StdEncoding.EncodeToString(png)})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (s *Server) publicInstanceStatus(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	state, err := s.engine.State(instanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) publicCheckContacts(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "WhatsApp engine is unavailable")
		return
	}
	var payload struct {
		Phones []string `json:"phones"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	result, err := s.engine.CheckContacts(r.Context(), instanceID, payload.Phones)
	if errors.Is(err, engine.ErrNotConnected) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}
