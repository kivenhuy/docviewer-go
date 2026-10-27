package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type errorResponse struct {
	Error string `json:"error"`
}

type Sales struct {
	SaleCode    string `json:"sale_code"`
	SaleNumber  string `json:"sale_number"`
	Title       string `json:"sale_title"`
	Type        string `json:"sale_type"`
	CreatedDate string `json:"created"`
}
type SaleResponse struct {
	VIN  string  `json:"vin"`
	Docs []Sales `json:"docs"`
}

var salesData = map[string][]Sales{
	"1HGCM82633A004352": {
		{SaleCode: "S-1", Title: "Sales contract", CreatedDate: "2024-07-01", SaleNumber: "D-99", Type: "contract"},
		{SaleCode: "S-2", Title: "Invoice", CreatedDate: "2024-07-02", SaleNumber: "D-99", Type: "invoice"},
	},
	"WAUZZZ8K9DA123456": {
		{SaleCode: "S-21", Title: "Invoice", CreatedDate: "2024-08-02", SaleNumber: "D-123", Type: "invoice"},
	},
}

func handler(w http.ResponseWriter, r *http.Request) {
	vin := r.URL.Query().Get("vin")
	if vin == "" {
		slog.Warn("request missing parameter", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "VIN is mandatory"})
		return
	}
	vin = strings.ToUpper(vin)
	if strings.HasPrefix(vin, "FAIL") {
		slog.Error("Sales system unavailable", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "sales system unavailable"})
		return
	}
	if strings.HasPrefix(vin, "SLOW") {
		slog.Info("request slow", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
		time.Sleep(5 * time.Second)
		slog.Info("request after 5 second", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
	}
	docs := salesData[vin]
	if docs == nil {
		docs = []Sales{}
	}
	resp := SaleResponse{
		VIN:  vin,
		Docs: docs,
	}
	slog.Info("request", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
	writeJSON(w, http.StatusOK, resp)

}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON error:", "err", err)
	}

}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sales/documents", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := ":" + port
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server err:", "err", err)
	}

}
