package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/outbox"
	"github.com/romariosribeiro/wirely-api/internal/storage"
)

func (s *Server) publicQueueText(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.queue == nil {
		writeError(w, http.StatusServiceUnavailable, "message queue is unavailable")
		return
	}
	var payload struct {
		Recipient   string `json:"recipient"`
		Message     string `json:"message"`
		ScheduledAt string `json:"scheduledAt"`
	}
	if err := decodeJSON(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	scheduledAt, err := parseScheduledAt(payload.ScheduledAt)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	job, replayed, err := s.queue.EnqueueText(r.Context(), instanceID, payload.Recipient, payload.Message, scheduledAt, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeQueueError(w, err)
		return
	}
	writeQueuedJob(w, job, replayed)
}

func (s *Server) publicQueueMedia(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.queue == nil {
		writeError(w, http.StatusServiceUnavailable, "message queue is unavailable")
		return
	}
	media, recipient, status, err := readMediaRequest(w, r)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	scheduledAt, err := parseScheduledAt(r.FormValue("scheduledAt"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	job, replayed, err := s.queue.EnqueueMedia(r.Context(), instanceID, recipient, media, scheduledAt, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeQueueError(w, err)
		return
	}
	writeQueuedJob(w, job, replayed)
}

func writeQueuedJob(w http.ResponseWriter, job storage.MessageJob, replayed bool) {
	w.Header().Set("Location", "/api/queue/"+job.ID)
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, job)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func parseScheduledAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("scheduledAt must be an RFC3339 date-time")
	}
	return parsed, nil
}

func writeQueueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, outbox.ErrInvalidIdempotencyKey), errors.Is(err, outbox.ErrScheduleTooFar):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
}

func (s *Server) publicGetMessageJob(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	s.getMessageJob(w, r, instanceID)
}

func (s *Server) getMessageJob(w http.ResponseWriter, r *http.Request, instanceID string) {
	jobID, ok := messageJobID(w, r)
	if !ok {
		return
	}
	job, err := s.store.GetMessageJob(r.Context(), instanceID, jobID)
	if errors.Is(err, storage.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read message job")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) publicCancelMessageJob(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	s.cancelMessageJobForInstance(w, r, instanceID)
}

func (s *Server) publicRetryMessageJob(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	s.retryMessageJobForInstance(w, r, instanceID)
}

func (s *Server) listMessageJobs(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	items, total, err := s.store.ListMessageJobs(r.Context(), instanceID, r.URL.Query().Get("status"), page, pageSize)
	if errors.Is(err, storage.ErrMessageJobFilter) {
		writeError(w, http.StatusBadRequest, "invalid message job status")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list message jobs")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(items, page, pageSize, total))
}

func (s *Server) cancelMessageJob(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	s.cancelMessageJobForInstance(w, r, instanceID)
}

func (s *Server) retryMessageJob(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	if !s.instanceExists(w, r, instanceID) {
		return
	}
	s.retryMessageJobForInstance(w, r, instanceID)
}

func (s *Server) cancelMessageJobForInstance(w http.ResponseWriter, r *http.Request, instanceID string) {
	if s.queue == nil {
		writeError(w, http.StatusServiceUnavailable, "message queue is unavailable")
		return
	}
	jobID, ok := messageJobID(w, r)
	if !ok {
		return
	}
	job, err := s.queue.Cancel(r.Context(), instanceID, jobID)
	if queueMutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) retryMessageJobForInstance(w http.ResponseWriter, r *http.Request, instanceID string) {
	if s.queue == nil {
		writeError(w, http.StatusServiceUnavailable, "message queue is unavailable")
		return
	}
	jobID, ok := messageJobID(w, r)
	if !ok {
		return
	}
	job, err := s.queue.Retry(r.Context(), instanceID, jobID)
	if queueMutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func queueMutationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storage.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
	} else if errors.Is(err, storage.ErrJobStateConflict) {
		writeError(w, http.StatusConflict, err.Error())
	} else {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
	return true
}

func messageJobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.PathValue("jobID"))
	if !strings.HasPrefix(id, "job_") || len(id) > 100 {
		writeError(w, http.StatusBadRequest, "invalid message job identifier")
		return "", false
	}
	return id, true
}
