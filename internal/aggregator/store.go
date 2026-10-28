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
