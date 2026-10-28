package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSalesClient_Fetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"vin":"1HGCM82633A004352","docs":[
		{"sale_code":"S-1","sale_title":"Sales contract","sale_type":"contract","created":"2024-07-01"},
		{"sale_code":"S-2","sale_title":"Invoice","sale_type":"invoice","created":"2024-07-02"}
	]}`))
	}))
	defer srv.Close()
	client := NewSalesClient(srv.URL, time.Second)
	docs, err := client.Fetch(context.Background(), "1HGCM82633A004352")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("data of docs = %d; want 2", len(docs))
	}
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
