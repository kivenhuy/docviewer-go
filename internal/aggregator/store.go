package aggregator

import (
	"context"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
)

type Snapshot struct {
	Documents []domain.Document
	FetchedAt time.Time
}

type SnapshotStore interface {
	Get(ctx context.Context, vin string) (map[string]Snapshot, error)
	Put(ctx context.Context, vin, source string, docs []domain.Document, fetchedAt time.Time) error
}

type CallLog struct {
	VIN      string
	Source   string
	Outcome  string // "ok" | "empty" | "error" | "timeout"
	Duration time.Duration
	Err      error // nil khi ok/empty
}

type CallLogger interface {
	LogCall(ctx context.Context, c CallLog) error
}
