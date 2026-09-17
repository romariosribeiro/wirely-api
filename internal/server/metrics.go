package server

import (
	"net/http"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

type metricsResponse struct {
	storage.MetricsSnapshot
	Range         string `json:"range"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "24h"
	}
	duration, ok := map[string]time.Duration{
		"24h": 24 * time.Hour,
		"7d":  7 * 24 * time.Hour,
		"30d": 30 * 24 * time.Hour,
	}[rangeName]
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be 24h, 7d, or 30d")
		return
	}

	now := time.Now().UTC()
	snapshot, err := s.store.Metrics(r.Context(), now.Add(-duration), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load metrics")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, metricsResponse{
		MetricsSnapshot: snapshot,
		Range:           rangeName,
		UptimeSeconds:   max(0, int64(time.Since(s.startedAt).Seconds())),
	})
}
