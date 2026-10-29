package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/aggregator"
	"github.com/kivenhuy/docviewer-go/internal/domain"
	"github.com/kivenhuy/docviewer-go/internal/reqid"
)

type fakePinger struct{ err error }

func (p fakePinger) PingContext(ctx context.Context) error { return p.err }

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		db         Pinger
		path       string
		wantStatus int
	}{
		{"healthz ignores the database", fakePinger{err: errors.New("down")}, "/healthz", http.StatusOK},
		{"readyz db up", fakePinger{}, "/readyz", http.StatusOK},
		{"readyz db down", fakePinger{err: errors.New("down")}, "/readyz", http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHealth(tt.db)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /healthz", h.Healthz)
			mux.HandleFunc("GET /readyz", h.Readyz)

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d; want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q; want application/json", ct)
			}
		})
	}
}

// Minimal fakes so the handler runs against a real Aggregator.
type oneSource struct{}

func (oneSource) Name() string { return "sales" }
func (oneSource) Fetch(ctx context.Context, vin string) ([]domain.Document, error) {
	return []domain.Document{{ID: "S-1", Title: "t", Type: "contract", Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Source: "sales"}}, nil
}

type nopStore struct{}

func (nopStore) Get(ctx context.Context, vin string) (map[string]aggregator.Snapshot, error) {
	return nil, nil
}
func (nopStore) Put(ctx context.Context, vin, source string, docs []domain.Document, at time.Time) error {
	return nil
}
func (nopStore) LogCall(ctx context.Context, c aggregator.CallLog) error { return nil }

func TestHandler_Documents(t *testing.T) {
	agg := aggregator.New([]domain.Source{oneSource{}}, time.Second, nopStore{}, 15*time.Minute, nopStore{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vehicles/{vin}/documents", NewHandler(agg).Documents)

	tests := []struct {
		name       string
		vin        string
		wantStatus int
		wantVIN    string
	}{
		{"valid", "1HGCM82633A004352", http.StatusOK, "1HGCM82633A004352"},
		{"lower case is normalised", "1hgcm82633a004352", http.StatusOK, "1HGCM82633A004352"},
		{"too short", "1HGCM826", http.StatusBadRequest, ""},
		{"bad character", "1HGCM82633A00435-", http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/vehicles/"+tt.vin+"/documents", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d; want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				var e errorResponse
				if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e.Error == "" {
					t.Errorf("want a JSON error body, got %q (decode err %v)", rec.Body, err)
				}
				return
			}
			var got aggregator.Result
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.VIN != tt.wantVIN || len(got.Documents) != 1 || got.Documents[0].Source != "sales" {
				t.Errorf("result = %+v; want VIN %s with one sales document", got, tt.wantVIN)
			}
		})
	}
}

// The middleware chain used in main.go: reqid outside, access log inside,
// so the access log line carries the request id and the real status code.
func TestAccessLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(reqid.NewLogHandler(slog.NewJSONHandler(&buf, nil))))
	defer slog.SetDefault(old)

	h := reqid.Middleware(AccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(reqid.Header, "trace-1")
	h.ServeHTTP(httptest.NewRecorder(), req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if line["status"] != float64(http.StatusTeapot) {
		t.Errorf("status = %v; want %d", line["status"], http.StatusTeapot)
	}
	if line["request_id"] != "trace-1" {
		t.Errorf("request_id = %v; want trace-1", line["request_id"])
	}
	if line["path"] != "/x" {
		t.Errorf("path = %v; want /x", line["path"])
	}
}
