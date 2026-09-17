package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type httpMetric struct {
	Method      string
	Route       string
	Status      int
	Requests    uint64
	DurationSum float64
}

type httpMetrics struct {
	mu       sync.Mutex
	items    map[string]httpMetric
	inFlight atomic.Int64
}

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{items: make(map[string]httpMetric)}
}

func (m *httpMetrics) record(method, route string, status int, duration time.Duration) {
	key := method + "\x00" + route + "\x00" + strconv.Itoa(status)
	m.mu.Lock()
	item := m.items[key]
	item.Method, item.Route, item.Status = method, route, status
	item.Requests++
	item.DurationSum += duration.Seconds()
	m.items[key] = item
	m.mu.Unlock()
}

func (m *httpMetrics) snapshot() []httpMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]httpMetric, 0, len(m.items))
	for _, item := range m.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Route != items[j].Route {
			return items[i].Route < items[j].Route
		}
		if items[i].Method != items[j].Method {
			return items[i].Method < items[j].Method
		}
		return items[i].Status < items[j].Status
	})
	return items
}

func (s *Server) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		s.httpMetrics.inFlight.Add(1)
		defer s.httpMetrics.inFlight.Add(-1)
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		duration := time.Since(started)
		s.httpMetrics.record(r.Method, route, status, duration)
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "http request",
			"method", r.Method, "route", route, "status", status,
			"duration_ms", float64(duration.Microseconds())/1000, "source_ip", clientIP(r))
	})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	snapshot, err := s.store.Metrics(r.Context(), now.Add(-30*24*time.Hour), now)
	if err != nil {
		http.Error(w, "failed to collect metrics", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, "# HELP wirely_up Whether the Wirely process is serving requests.")
	fmt.Fprintln(w, "# TYPE wirely_up gauge")
	fmt.Fprintln(w, "wirely_up 1")
	fmt.Fprintln(w, "# HELP wirely_uptime_seconds Process uptime in seconds.")
	fmt.Fprintln(w, "# TYPE wirely_uptime_seconds gauge")
	fmt.Fprintf(w, "wirely_uptime_seconds %d\n", max(0, int64(time.Since(s.startedAt).Seconds())))
	fmt.Fprintln(w, "# HELP wirely_instances Current instance count by connection status.")
	fmt.Fprintln(w, "# TYPE wirely_instances gauge")
	statuses := map[string]int{}
	for _, item := range snapshot.ByInstance {
		statuses[item.Status]++
	}
	for _, status := range sortedKeys(statuses) {
		fmt.Fprintf(w, "wirely_instances{status=%s} %d\n", promQuote(status), statuses[status])
	}
	fmt.Fprintln(w, "# HELP wirely_messages_total Messages observed in the retained 30-day window.")
	fmt.Fprintln(w, "# TYPE wirely_messages_total gauge")
	fmt.Fprintf(w, "wirely_messages_total{direction=\"sent\"} %d\n", snapshot.Messages.Sent)
	fmt.Fprintf(w, "wirely_messages_total{direction=\"received\"} %d\n", snapshot.Messages.Received)
	fmt.Fprintln(w, "# HELP wirely_queue_jobs Current or retained queue jobs by status.")
	fmt.Fprintln(w, "# TYPE wirely_queue_jobs gauge")
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"queued\"} %d\n", snapshot.Queue.Queued)
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"processing\"} %d\n", snapshot.Queue.Processing)
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"retrying\"} %d\n", snapshot.Queue.Retrying)
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"sent\"} %d\n", snapshot.Queue.Sent)
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"failed\"} %d\n", snapshot.Queue.Failed)
	fmt.Fprintf(w, "wirely_queue_jobs{status=\"canceled\"} %d\n", snapshot.Queue.Canceled)
	fmt.Fprintln(w, "# HELP wirely_webhook_deliveries_total Webhook deliveries in the retained 30-day window.")
	fmt.Fprintln(w, "# TYPE wirely_webhook_deliveries_total gauge")
	fmt.Fprintf(w, "wirely_webhook_deliveries_total{status=\"delivered\"} %d\n", snapshot.Webhooks.Delivered)
	fmt.Fprintf(w, "wirely_webhook_deliveries_total{status=\"failed\"} %d\n", snapshot.Webhooks.Failed)
	fmt.Fprintln(w, "# HELP wirely_http_requests_total HTTP requests handled by method, route, and status.")
	fmt.Fprintln(w, "# TYPE wirely_http_requests_total counter")
	fmt.Fprintln(w, "# HELP wirely_http_request_duration_seconds_sum Cumulative HTTP request duration.")
	fmt.Fprintln(w, "# TYPE wirely_http_request_duration_seconds_sum counter")
	for _, item := range s.httpMetrics.snapshot() {
		labels := fmt.Sprintf("method=%s,route=%s,status=%s", promQuote(item.Method), promQuote(item.Route), promQuote(strconv.Itoa(item.Status)))
		fmt.Fprintf(w, "wirely_http_requests_total{%s} %d\n", labels, item.Requests)
		fmt.Fprintf(w, "wirely_http_request_duration_seconds_sum{%s} %.6f\n", labels, item.DurationSum)
	}
	fmt.Fprintln(w, "# HELP wirely_http_requests_in_flight HTTP requests currently being served.")
	fmt.Fprintln(w, "# TYPE wirely_http_requests_in_flight gauge")
	fmt.Fprintf(w, "wirely_http_requests_in_flight %d\n", s.httpMetrics.inFlight.Load())
}

type Alert struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	InstanceID string `json:"instanceId,omitempty"`
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	snapshot, err := s.store.Metrics(r.Context(), now.Add(-24*time.Hour), now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to evaluate alerts")
		return
	}
	alerts := make([]Alert, 0)
	for _, item := range snapshot.ByInstance {
		if item.Status == "error" {
			alerts = append(alerts, Alert{ID: "instance-error-" + item.ID, Severity: "critical", Title: "Instância com erro", Message: item.Name + " exige atenção.", InstanceID: item.ID})
		}
		if item.Queue.Failed > 0 {
			alerts = append(alerts, Alert{ID: "queue-failed-" + item.ID, Severity: "warning", Title: "Falhas na fila", Message: fmt.Sprintf("%s teve %d envio(s) com falha nas últimas 24 horas.", item.Name, item.Queue.Failed), InstanceID: item.ID})
		}
		if item.Webhooks.Total >= 5 && item.Webhooks.SuccessRate < 90 {
			alerts = append(alerts, Alert{ID: "webhook-rate-" + item.ID, Severity: "warning", Title: "Webhook instável", Message: fmt.Sprintf("%s está com %.1f%% de sucesso nas últimas 24 horas.", item.Name, item.Webhooks.SuccessRate), InstanceID: item.ID})
		}
	}
	if s.backups != nil {
		status, backupErr := s.backups.Status(r.Context())
		if backupErr != nil {
			alerts = append(alerts, Alert{ID: "backup-unavailable", Severity: "critical", Title: "Backup indisponível", Message: "Não foi possível verificar os backups automáticos."})
		} else if status.Automatic && !status.NextBackupAt.IsZero() && now.After(status.NextBackupAt.Add(15*time.Minute)) {
			alerts = append(alerts, Alert{ID: "backup-overdue", Severity: "warning", Title: "Backup atrasado", Message: "O backup automático não executou no horário esperado."})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": alerts, "total": len(alerts), "generatedAt": now})
}

func promQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
