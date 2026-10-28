package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceClient_Fetch(t *testing.T) {
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
	 	{"ticket_id":"T-1001","description":"Oil change","service_date":"2024-07-04","mileage":15000,"workshop":"Downtown Garage"},
	 	{"ticket_id":"T-1002","description":"Brake pad replacement","service_date":"2024-09-15","mileage":21000,"workshop":"Downtown Garage"} ]}`, wantLen: 2},
		{name: "empty", status: 200, body: `{"vin":"X","docs":[]}`, wantLen: 0},
		{name: "server error", status: 500, wantErr: true},
		{name: "bad date", status: 200, body: `{"vin":"1HGCM82633A004352","docs":[
	 	{"ticket_id":"T-1001","description":"Oil change","service_date":"hello","mileage":15000,"workshop":"Downtown Garage"} ]}`, wantErr: true},
		{name: "timeout", status: 200, body: `{"vin":"X","docs":[]}`, delay: 200 * time.Millisecond, ctxTimeout: 50 * time.Millisecond, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvtest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/service/documents" {
					t.Errorf("path = %s", r.URL.Path)
				}
				if got := r.URL.Query().Get("vin"); got != "1HGCM82633A004352" {
					t.Errorf("vin = %q", got)
				}
				if tt.delay > 0 {
					time.Sleep(tt.delay)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srvtest.Close()
			ctx := context.Background()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}

			client := NewServiceClient(srvtest.URL, time.Second)
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
				if docs[0].ID != "T-1001" {
					t.Errorf("ID of first docs = %v; want T-1001", docs[0].ID)
				}
				if docs[0].Title != "Oil change" {
					t.Errorf("Title of first docs = %v; want Oil change", docs[0].Title)
				}
				if docs[0].Source != "service" {
					t.Errorf("Source of first docs = %v; want service", docs[0].Source)
				}
				if docs[0].Type != "service_ticket" {
					t.Errorf("Type of first docs = %v; want service_ticket", docs[0].Type)
				}
				if !docs[0].Date.Equal(time.Date(2024, 7, 4, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("Date of first docs = %v;", docs[0].Date)
				}
			}
		})
	}
}
