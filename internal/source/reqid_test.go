package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
	"github.com/kivenhuy/docviewer-go/internal/reqid"
)

// Both clients forward the request id from the context in X-Request-ID,
// and send no header when the context has no id.
func TestClients_PropagateRequestID(t *testing.T) {
	clients := map[string]func(baseURL string) domain.Source{
		"sales":   func(u string) domain.Source { return NewSalesClient(u, time.Second) },
		"service": func(u string) domain.Source { return NewServiceClient(u, time.Second) },
	}
	for name, newClient := range clients {
		t.Run(name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get(reqid.Header)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"vin":"X","docs":[]}`))
			}))
			defer srv.Close()
			c := newClient(srv.URL)

			if _, err := c.Fetch(reqid.With(context.Background(), "abc123"), "X"); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if got != "abc123" {
				t.Errorf("X-Request-ID = %q; want abc123", got)
			}

			if _, err := c.Fetch(context.Background(), "X"); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if got != "" {
				t.Errorf("X-Request-ID = %q; want no header without an id", got)
			}
		})
	}
}
