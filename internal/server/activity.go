package server

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/romariosribeiro/wirely-api/internal/engine"
	"github.com/romariosribeiro/wirely-api/internal/storage"
	"github.com/romariosribeiro/wirely-api/internal/webhook"
)

type activityPage struct {
	Data       any `json:"data"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

func (s *Server) listActivityEvents(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	items, total, err := s.store.ListActivityEvents(r.Context(), instanceID, r.URL.Query().Get("category"), page, pageSize)
	if errors.Is(err, storage.ErrActivityFilter) {
		writeError(w, http.StatusBadRequest, "invalid activity category")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(items, page, pageSize, total))
}

func (s *Server) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
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

func (s *Server) retryWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	deliveryID, err := strconv.ParseInt(r.PathValue("deliveryID"), 10, 64)
	if err != nil || deliveryID < 1 {
		writeError(w, http.StatusBadRequest, "invalid delivery identifier")
		return
	}
	delivery, err := s.store.GetWebhookDelivery(r.Context(), instanceID, deliveryID)
	if errors.Is(err, storage.ErrDeliveryNotFound) {
		writeError(w, http.StatusNotFound, "webhook delivery not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read webhook delivery")
		return
	}
	if delivery.Status != "failed" {
		writeError(w, http.StatusConflict, "only failed deliveries can be retried")
		return
	}
	if s.webhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook dispatcher unavailable")
		return
	}
	storedEvent, err := s.store.GetActivityEvent(r.Context(), instanceID, delivery.EventID)
	if errors.Is(err, storage.ErrActivityEventMissing) {
		writeError(w, http.StatusNotFound, "original event is no longer available")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read original event")
		return
	}
	err = s.webhooks.Retry(engine.Event{
		ID: storedEvent.ID, Event: storedEvent.Event, InstanceID: storedEvent.InstanceID,
		Timestamp: storedEvent.Timestamp, Data: storedEvent.Data,
	})
	if errors.Is(err, webhook.ErrWebhookUnavailable) {
		writeError(w, http.StatusConflict, "webhook is disabled or does not accept this event")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "webhook retry failed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "eventId": storedEvent.ID})
}

func (s *Server) instanceExists(w http.ResponseWriter, r *http.Request, instanceID string) bool {
	_, err := s.store.GetInstance(r.Context(), instanceID)
	if errors.Is(err, storage.ErrInstanceNotFound) {
		writeError(w, http.StatusNotFound, "instance not found")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read instance")
		return false
	}
	return true
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, pageSize := 1, 25
	var err error
	if value := r.URL.Query().Get("page"); value != "" {
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 {
			writeError(w, http.StatusBadRequest, "page must be a positive integer")
			return 0, 0, false
		}
	}
	if value := r.URL.Query().Get("pageSize"); value != "" {
		pageSize, err = strconv.Atoi(value)
		if err != nil || pageSize < 1 || pageSize > 100 {
			writeError(w, http.StatusBadRequest, "pageSize must be between 1 and 100")
			return 0, 0, false
		}
	}
	return page, pageSize, true
}

func pageResponse(data any, page, pageSize, total int) activityPage {
	return activityPage{Data: data, Page: page, PageSize: pageSize, Total: total,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize)))}
}
