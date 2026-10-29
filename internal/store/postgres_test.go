package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver for database/sql

	"github.com/kivenhuy/docviewer-go/internal/aggregator"
	"github.com/kivenhuy/docviewer-go/internal/domain"
)

// testDB is shared by every integration test in this package.
// nil means Postgres is not reachable: tests skip instead of failing.
var testDB *sql.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/docviewer?sslmode=disable"
	}
	if db, err := sql.Open("pgx", dsn); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := db.PingContext(ctx); err == nil {
			testDB = db
		} else {
			db.Close()
		}
		cancel()
	}
	code := m.Run()
	if testDB != nil {
		testDB.Close()
	}
	os.Exit(code)
}

func requireDB(t *testing.T) *PostgresStore {
	t.Helper()
	if testDB == nil {
		t.Skip("no database: set DATABASE_URL or start Postgres on localhost:5432")
	}
	return NewPostgresStore(testDB)
}

func cleanVIN(t *testing.T, vin string) {
	t.Helper()
	if _, err := testDB.ExecContext(context.Background(), `DELETE FROM document_snapshots WHERE vin = $1`, vin); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

func testDoc(id string) domain.Document {
	return domain.Document{ID: id, Title: id, Type: "x", Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Source: "sales"}
}

// Postgres stores timestamptz with microsecond precision; Go has nanoseconds.
// Truncate before writing so the value read back is Equal to the value written.
func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func TestPostgresStore_PutThenGet(t *testing.T) {
	s := requireDB(t)
	const vin = "TEST-PUT-GET"
	cleanVIN(t, vin)
	ctx := context.Background()

	fetched := now()
	docs := []domain.Document{testDoc("S-1"), testDoc("S-2")}
	if err := s.Put(ctx, vin, "sales", docs, fetched); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get(ctx, vin)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	snap, ok := got["sales"]
	if !ok {
		t.Fatalf("Get: missing key sales, got keys %v", keys(got))
	}
	if len(snap.Documents) != 2 {
		t.Fatalf("len(Documents) = %d; want 2", len(snap.Documents))
	}
	if snap.Documents[0].ID != "S-1" {
		t.Errorf("Documents[0].ID = %q; want S-1", snap.Documents[0].ID)
	}
	if !snap.FetchedAt.Equal(fetched) {
		t.Errorf("FetchedAt = %v; want %v", snap.FetchedAt, fetched)
	}
}

func TestPostgresStore_UpsertOverwrites(t *testing.T) {
	s := requireDB(t)
	const vin = "TEST-UPSERT"
	cleanVIN(t, vin)
	ctx := context.Background()

	if err := s.Put(ctx, vin, "sales", []domain.Document{testDoc("OLD")}, now()); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second := now()
	if err := s.Put(ctx, vin, "sales", []domain.Document{testDoc("A"), testDoc("B"), testDoc("C")}, second); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	got, err := s.Get(ctx, vin)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(Get) = %d; want 1 entry for (vin, sales)", len(got))
	}
	snap := got["sales"]
	if len(snap.Documents) != 3 {
		t.Errorf("len(Documents) = %d; want 3 (second Put must overwrite)", len(snap.Documents))
	}
	if !snap.FetchedAt.Equal(second) {
		t.Errorf("FetchedAt = %v; want %v (second Put)", snap.FetchedAt, second)
	}
}

func TestPostgresStore_EmptySnapshotSurvives(t *testing.T) {
	s := requireDB(t)
	const vin = "TEST-EMPTY"
	cleanVIN(t, vin)
	ctx := context.Background()

	if err := s.Put(ctx, vin, "service", nil, now()); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, vin)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	snap, ok := got["service"]
	if !ok {
		t.Fatalf("empty snapshot must still be returned as an entry; got keys %v", keys(got))
	}
	if snap.Documents == nil {
		t.Errorf("Documents = nil; want empty slice (JSON [] not null)")
	}
	if len(snap.Documents) != 0 {
		t.Errorf("len(Documents) = %d; want 0", len(snap.Documents))
	}
}

func TestPostgresStore_LogCall(t *testing.T) {
	s := requireDB(t)
	const vin = "TEST-CALL-LOG"
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `DELETE FROM source_call_log WHERE vin = $1`, vin); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	calls := []aggregator.CallLog{
		{VIN: vin, Source: "sales", Outcome: "ok", Duration: 120 * time.Millisecond},
		{VIN: vin, Source: "service", Outcome: "timeout", Duration: 3 * time.Second, Err: errors.New("context deadline exceeded")},
	}
	for _, c := range calls {
		if err := s.LogCall(ctx, c); err != nil {
			t.Fatalf("LogCall(%s): %v", c.Source, err)
		}
	}

	rows, err := testDB.QueryContext(ctx,
		`SELECT source, outcome, duration_ms, error FROM source_call_log WHERE vin = $1 ORDER BY source`, vin)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	type row struct {
		source, outcome string
		ms              int64
		errStr          sql.NullString
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.source, &r.outcome, &r.ms, &r.errStr); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("rows = %d; want 2", len(got))
	}
	if got[0].source != "sales" || got[0].outcome != "ok" || got[0].ms != 120 || got[0].errStr.Valid {
		t.Errorf("sales row = %+v; want ok, 120ms, NULL error", got[0])
	}
	if got[1].source != "service" || got[1].outcome != "timeout" || got[1].ms != 3000 ||
		got[1].errStr.String != "context deadline exceeded" {
		t.Errorf("service row = %+v; want timeout, 3000ms, error text stored", got[1])
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
