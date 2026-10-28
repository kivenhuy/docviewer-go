package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSalesClient_Fetch(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		delay      time.Duration // handler sleep time
		ctxTimeout time.Duration // if 0 is no timeout
		wantErr    bool
		wantLen    int
	}{
		{name: "happy", status: 200, body: `{"vin":"1HGCM82633A004352","docs":[
	 	{"sale_code":"S-1","sale_title":"Sales contract","sale_type":"contract","created":"2024-07-01"},
	 	{"sale_code":"S-2","sale_title":"Invoice","sale_type":"invoice","created":"2024-07-02"} ]}`, wantLen: 2},
		{name: "empty", status: 200, body: `{"vin":"X","docs":[]}`, wantLen: 0},
		{name: "server error", status: 500, wantErr: true},
		{name: "bad date", status: 200, body: `{"vin":"1HGCM82633A004352","docs":[
	 	{"sale_code":"S-1","sale_title":"Sales contract","sale_type":"contract","created":"hello"} ]}`, wantErr: true},
		{name: "timeout", status: 200, body: `{"vin":"X","docs":[]}`, delay: 200 * time.Millisecond, ctxTimeout: 50 * time.Millisecond, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. NewServer with handler read tt.status, tt.body, tt.delay
			srvtest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.delay > 0 {
					time.Sleep(tt.delay)
				}
				if r.URL.Path != "/sales/documents" {
					t.Errorf("path = %s", r.URL.Path)
				}
				if got := r.URL.Query().Get("vin"); got != "1HGCM82633A004352" {
					t.Errorf("vin = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))

			}))
			defer srvtest.Close()

			ctx := context.Background()
			// 2 Check timeout
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}

			client := NewSalesClient(srvtest.URL, time.Second)
			// 3 Fetch data
			docs, err := client.Fetch(ctx, "1HGCM82633A004352")
			if tt.ctxTimeout > 0 && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Fetch error: %v", err)
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("Fetch error: want: %v, err: %v", tt.wantErr, err)
			}
			if !tt.wantErr && len(docs) != tt.wantLen {
				t.Fatalf("data of docs = %d; want %d", len(docs), tt.wantLen)
			}
			if tt.name == "happy" {
				if docs[0].ID != "S-1" {
					t.Errorf("ID of first docs = %v; want S-1", docs[0].ID)
				}
				if docs[0].Source != "sales" {
					t.Errorf("Source of first docs = %v; want sales", docs[0].Source)
				}
				if docs[0].Type != "contract" {
					t.Errorf("Type of first docs = %v; want contract", docs[0].Type)
				}
				if !docs[0].Date.Equal(time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("Date of first docs = %v;", docs[0].Date)
				}
			}

		})
	}

}
