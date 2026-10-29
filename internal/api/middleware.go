package api

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder remembers the status code written by the handler,
// so the access log can report it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// quietPaths are probed every few seconds by the orchestrator (Compose healthcheck,
// Kubernetes probes). Logging them would bury the real traffic.
var quietPaths = map[string]bool{"/healthz": true, "/readyz": true}

// AccessLog writes one log line per HTTP request: method, path, status, duration.
// It must run inside reqid.Middleware so the line carries the request_id.
// Health probes are not logged; a failing /readyz still logs its own warning.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if quietPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK} // no WriteHeader call means 200
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
