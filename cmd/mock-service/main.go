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

type ServiceDoc struct {
	TicketID    string `json:"ticket_id"`
	Description string `json:"description"`
	ServiceDate string `json:"service_date"`
	Mileage     int    `json:"mileage"`
	Workshop    string `json:"workshop"`
}
type ServiceResponse struct {
	VIN  string       `json:"vin"`
	Docs []ServiceDoc `json:"docs"`
}

var serviceData = map[string][]ServiceDoc{
	// VIN match with mock Sales → test data have in 2 system
	"1HGCM82633A004352": {
		{TicketID: "T-1001", Description: "Oil change", ServiceDate: "2024-07-04", Mileage: 15000, Workshop: "Downtown Garage"},
		{TicketID: "T-1002", Description: "Brake pad replacement", ServiceDate: "2024-09-15", Mileage: 21000, Workshop: "Downtown Garage"},
	},
	// VIN only Service have data → test Case Sales null
	"JH4KA7532NC036794": {
		{TicketID: "T-2001", Description: "Annual inspection", ServiceDate: "2024-05-20", Mileage: 48000, Workshop: "Northside Service"},
	},
}

func handler(w http.ResponseWriter, r *http.Request) {
	// request_id comes from the API in X-Request-ID, so one id can be grepped across all three services.
	log := slog.With("request_id", r.Header.Get("X-Request-ID"))
	vin := r.URL.Query().Get("vin")
	if vin == "" {
		log.Warn("request missing parameter", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "VIN is mandatory"})
		return
	}
	vin = strings.ToUpper(vin)
	if strings.HasPrefix(vin, "FAIL") {
		log.Error("Service system unavailable", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusInternalServerError)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "service system unavailable"})
		return
	}
	if strings.HasPrefix(vin, "SLOW") {
		log.Info("request slow", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
		time.Sleep(5 * time.Second)
		log.Info("request after 5 second", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
	}
	docs := serviceData[vin]
	if docs == nil {
		docs = []ServiceDoc{}
	}
	resp := ServiceResponse{
		VIN:  vin,
		Docs: docs,
	}
	log.Info("request", "method", r.Method, "path", r.URL.Path, "vin", vin, "status", http.StatusOK)
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
	mux.HandleFunc("GET /service/documents", handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
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
