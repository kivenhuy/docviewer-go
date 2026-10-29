package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/aggregator"
	"github.com/kivenhuy/docviewer-go/internal/domain"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

var _ aggregator.SnapshotStore = (*PostgresStore)(nil)
var _ aggregator.CallLogger = (*PostgresStore)(nil)

func (s *PostgresStore) Get(ctx context.Context, vin string) (map[string]aggregator.Snapshot, error) {
	const q = `
		SELECT source, documents, fetched_at
		FROM document_snapshots
		WHERE vin = $1
	`
	rows, err := s.db.QueryContext(ctx, q, vin)
	if err != nil {
		return nil, fmt.Errorf("query document_snapshots: %w", err)
	}
	defer rows.Close()
	result := make(map[string]aggregator.Snapshot)

	for rows.Next() {
		var (
			source    string
			documents []byte
			FetchedAt time.Time
		)

		if err := rows.Scan(&source, &documents, &FetchedAt); err != nil {
			return nil, fmt.Errorf("scan document_snapshots: %w", err)
		}
		var docs []domain.Document
		if err := json.Unmarshal(documents, &docs); err != nil {
			return nil, fmt.Errorf("decode snapshot %s/%s: %w", vin, source, err)
		}
		result[source] = aggregator.Snapshot{Documents: docs, FetchedAt: FetchedAt}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document_snapshots: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) Put(ctx context.Context, vin, source string, docs []domain.Document, fetchedAt time.Time) error {
	if docs == nil {
		docs = []domain.Document{}
	}
	raw, err := json.Marshal(docs)
	if err != nil {
		return fmt.Errorf("encode documents: %w", err)
	}
	const q = `
		INSERT INTO document_snapshots (vin, source, documents, fetched_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vin, source)
		DO UPDATE SET documents = EXCLUDED.documents, fetched_at = EXCLUDED.fetched_at
	
	`

	_, err = s.db.ExecContext(ctx, q, vin, source, raw, fetchedAt)
	if err != nil {
		return fmt.Errorf("update document_snapshots: %w", err)
	}

	return nil
}

func (s *PostgresStore) LogCall(ctx context.Context, c aggregator.CallLog) error {
	const q = `INSERT INTO source_call_log (vin, source, outcome, duration_ms, error) VALUES ($1, $2, $3, $4, $5)`
	var errStr sql.NullString
	if c.Err != nil {
		errStr = sql.NullString{String: c.Err.Error(), Valid: true}
	}
	_, err := s.db.ExecContext(ctx, q, c.VIN, c.Source, c.Outcome, c.Duration.Milliseconds(), errStr)
	if err != nil {
		return fmt.Errorf("insert source_call_log: %w", err)
	}
	return nil
}
