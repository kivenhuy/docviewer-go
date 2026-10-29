package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// Pinger is the one method /readyz needs from the database; *sql.DB satisfies it.
type Pinger interface {
	PingContext(ctx context.Context) error
}

type Health struct {
	db Pinger
}

func NewHealth(db Pinger) *Health {
	return &Health{db: db}
}

// Healthz is liveness: the process is up and serving HTTP. No dependency is checked,
// so a database outage does not make the orchestrator restart a healthy process.
func (h *Health) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz is readiness: Postgres must be reachable. The Sales and Service systems are
// deliberately not checked; when they are down the API still answers from snapshots.
func (h *Health) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.db.PingContext(ctx); err != nil {
		slog.WarnContext(r.Context(), "readiness check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "postgres": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "postgres": "up"})
}
