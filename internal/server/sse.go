package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) publicEvents(w http.ResponseWriter, r *http.Request) {
	instanceID, ok := s.authenticateInstanceRequest(w, r)
	if !ok {
		return
	}
	if s.events == nil {
		writeError(w, http.StatusServiceUnavailable, "event stream is unavailable")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is unsupported")
		return
	}
	filter := eventFilter(r.URL.Query().Get("events"))
	channel, cancel := s.events.Subscribe(instanceID)
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, open := <-channel:
			if !open {
				return
			}
			if len(filter) > 0 {
				if _, wanted := filter[event.Event]; !wanted {
					continue
				}
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", cleanSSELine(event.ID), cleanSSELine(event.Event), data)
			flusher.Flush()
		}
	}
}

func eventFilter(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result[item] = struct{}{}
		}
	}
	return result
}

func cleanSSELine(value string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(value) }
