package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/kivenhuy/docviewer-go/internal/aggregator"
	"github.com/kivenhuy/docviewer-go/internal/api"
	"github.com/kivenhuy/docviewer-go/internal/domain"
	"github.com/kivenhuy/docviewer-go/internal/source"
	"github.com/kivenhuy/docviewer-go/internal/store"
)

const (
	sourceTimeout = 3 * time.Second  // per external source, see design.md §5
	snapshotTTL   = 15 * time.Minute // cache TTL, see design.md §5
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dsn := envOr("DATABASE_URL", "postgres://localhost:5432/docviewer?sslmode=disable")
	salesURL := envOr("SALES_URL", "http://localhost:8081")
	serviceURL := envOr("SERVICE_URL", "http://localhost:8082")
	addr := ":" + envOr("PORT", "8080")

	// Postgres: snapshot cache + audit log.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		slog.Error("ping database", "err", err)
		os.Exit(1)
	}

	// Wiring: sources -> aggregator -> handler -> mux -> server.
	sources := []domain.Source{
		source.NewSalesClient(salesURL, sourceTimeout),
		source.NewServiceClient(serviceURL, sourceTimeout),
	}
	agg := aggregator.New(sources, sourceTimeout, store.NewPostgresStore(db), snapshotTTL)
	h := api.NewHandler(agg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/vehicles/{vin}/documents", h.Documents)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second, // > sourceTimeout, so a slow source still gets a response out
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("server starting", "addr", addr, "sales_url", salesURL, "service_url", serviceURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
