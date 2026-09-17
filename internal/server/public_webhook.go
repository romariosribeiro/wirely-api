package server

import (
	"errors"
	"net/http"

	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/webhook"
)

func (s *Server) publicTestWebhook(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.webhookTester == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook dispatcher unavailable")
		return
	}
	eventID, err := s.webhookTester.Test(instanceID)
	if errors.Is(err, webhook.ErrWebhookUnavailable) {
		writeError(w, http.StatusConflict, "webhook is not configured or is disabled")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue webhook test")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"eventId": eventID, "status": "queued"})
}

func (s *Server) publicListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	items, total, err := s.store.ListWebhookDeliveries(r.Context(), instanceID, r.URL.Query().Get("status"), page, pageSize)
	if errors.Is(err, storage.ErrActivityFilter) {
		writeError(w, http.StatusBadRequest, "invalid delivery status")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list webhook deliveries")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(items, page, pageSize, total))
}

func (s *Server) publicGetWebhookJob(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetWebhookJob(r.Context(), instanceID, r.PathValue("eventID"))
	if errors.Is(err, storage.ErrWebhookJobNotFound) {
		writeError(w, http.StatusNotFound, "webhook job not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read webhook job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}
