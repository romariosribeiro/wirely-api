package server

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/romariosribeiro/wirely-api/internal/storage"
)

const defaultAPIRateLimit = 120

type rateBucket struct {
	started time.Time
	count   int
}

type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	items  map[string]rateBucket
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit < 1 {
		limit = defaultAPIRateLimit
	}
	if window <= 0 {
		window = time.Minute
	}
	return &rateLimiter{limit: limit, window: window, items: make(map[string]rateBucket)}
}

func (l *rateLimiter) allow(key string, now time.Time) (bool, int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, exists := l.items[key]
	if !exists || !now.Before(bucket.started.Add(l.window)) {
		bucket = rateBucket{started: now, count: 0}
	}
	bucket.count++
	l.items[key] = bucket
	remaining := l.limit - bucket.count
	if remaining < 0 {
		remaining = 0
	}
	return bucket.count <= l.limit, remaining, bucket.started.Add(l.window)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (s *Server) auditAction(action, targetType, pathKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		user, _ := currentUser(r)
		entry := storage.AuditEntry{
			UserID: user.ID, Username: user.Username, Action: action,
			TargetType: targetType, TargetID: r.PathValue(pathKey), Status: status,
			SourceIP: clientIP(r), Details: map[string]any{"method": r.Method, "path": r.URL.Path},
		}
		if err := s.store.RecordAudit(r.Context(), entry); err != nil {
			slog.Error("failed to record admin audit", "action", action, "error", err)
		}
	})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := pagination(w, r)
	if !ok {
		return
	}
	entries, total, err := s.store.ListAudit(r.Context(), page, pageSize, r.URL.Query().Get("action"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit entries")
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(entries, page, pageSize, total))
}

func clientIP(r *http.Request) string {
	value := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	return value
}

func setRateLimitHeaders(w http.ResponseWriter, limit, remaining int, reset time.Time) {
	w.Header().Set("RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
}

func retryAfterSeconds(until time.Time) int {
	seconds := int(time.Until(until).Seconds()) + 1
	if seconds < 1 {
		return 1
	}
	return seconds
}

func writeRateLimitExceeded(w http.ResponseWriter, reset time.Time) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(reset)))
	writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
}

func writeLoginLocked(w http.ResponseWriter, lockedUntil time.Time) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(lockedUntil)))
	writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
}
