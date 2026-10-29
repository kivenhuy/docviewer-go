package reqid

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantKeep bool // true: the incoming id must be reused
	}{
		{name: "no header: new id", incoming: ""},
		{name: "valid header: reused", incoming: "gw-123.abc_DEF", wantKeep: true},
		{name: "header with spaces: replaced", incoming: "evil id\nfake log line"},
		{name: "header too long: replaced", incoming: string(bytes.Repeat([]byte("a"), 65))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = From(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(Header, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if !validID.MatchString(seen) {
				t.Fatalf("id in context = %q; want a valid id", seen)
			}
			if got := rec.Header().Get(Header); got != seen {
				t.Errorf("response header = %q; want the context id %q", got, seen)
			}
			if tt.wantKeep && seen != tt.incoming {
				t.Errorf("id = %q; want incoming %q reused", seen, tt.incoming)
			}
			if !tt.wantKeep && seen == tt.incoming {
				t.Errorf("id = %q; want a new id", seen)
			}
		})
	}
}

func TestMiddleware_NewIDsAreUnique(t *testing.T) {
	ids := map[string]bool{}
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids[From(r.Context())] = true
	}))
	for range 100 {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}
	if len(ids) != 100 {
		t.Errorf("got %d distinct ids for 100 requests; want 100", len(ids))
	}
}

func TestLogHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewLogHandler(slog.NewJSONHandler(&buf, nil)))
	ctx := With(context.Background(), "abc")

	read := func() map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
		}
		buf.Reset()
		return m
	}

	logger.InfoContext(ctx, "with ctx")
	if got := read()["request_id"]; got != "abc" {
		t.Errorf("request_id = %v; want abc", got)
	}

	logger.With("vin", "X").InfoContext(ctx, "after With")
	if got := read()["request_id"]; got != "abc" {
		t.Errorf("request_id after With = %v; want abc (WithAttrs must keep the wrapper)", got)
	}

	logger.Info("no ctx")
	if _, ok := read()["request_id"]; ok {
		t.Errorf("request_id present without a context id; want absent")
	}
}
