// Package reqid gives every HTTP request an id, carries it in context.Context,
// adds it to every log line written with a context, and forwards it to
// downstream systems in the X-Request-ID header.
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
)

const Header = "X-Request-ID"

type ctxKey struct{}

// validID limits ids accepted from the caller: no spaces or control characters
// (they end up in logs), and a bounded length.
var validID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// With returns a copy of ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the id stored in ctx, or "" if there is none.
func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

func newID() string {
	b := make([]byte, 16) // 128 bits, same size as a UUID
	_, _ = rand.Read(b)   // crypto/rand.Read never returns an error (Go 1.24+)
	return hex.EncodeToString(b)
}

// Middleware reuses a valid incoming X-Request-ID (so a gateway's id is kept)
// or creates a new one, stores it in the request context and echoes it in the
// response header.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Header)
		if !validID.MatchString(id) {
			id = newID()
		}
		w.Header().Set(Header, id)
		next.ServeHTTP(w, r.WithContext(With(r.Context(), id)))
	})
}

// LogHandler wraps a slog.Handler and adds request_id to every record logged
// with a context that carries one (slog.InfoContext, slog.WarnContext, ...).
type LogHandler struct {
	slog.Handler
}

func NewLogHandler(h slog.Handler) *LogHandler { return &LogHandler{Handler: h} }

func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := From(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs and WithGroup must return a wrapped handler too,
// otherwise slog.With(...) would silently drop the request_id.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithGroup(name)}
}
