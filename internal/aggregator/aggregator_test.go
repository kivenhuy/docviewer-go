package aggregator

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
)

// fakeSource satisfies domain.Source without any HTTP.
// This is the payoff of the Source interface: the aggregator can be tested in isolation.
type fakeSource struct {
	name  string
	docs  []domain.Document
	err   error
	delay time.Duration
	calls *int32 // optional: counts Fetch calls (atomic, Fetch runs in goroutines)
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(ctx context.Context, vin string) ([]domain.Document, error) {
	if f.calls != nil {
		atomic.AddInt32(f.calls, 1)
	}
	if f.delay == 0 {
		return f.docs, f.err
	}
	select {
	case <-time.After(f.delay):
		return f.docs, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// fakeStore is an in-memory SnapshotStore: vin -> source -> Snapshot.
// It also counts Put calls so tests can assert when the aggregator writes back.
type fakeStore struct {
	snaps map[string]map[string]Snapshot
	puts  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{snaps: map[string]map[string]Snapshot{}}
}

func (s *fakeStore) Get(ctx context.Context, vin string) (map[string]Snapshot, error) {
	out := map[string]Snapshot{}
	for src, snap := range s.snaps[vin] {
		out[src] = snap
	}
	return out, nil
}

func (s *fakeStore) Put(ctx context.Context, vin, source string, docs []domain.Document, fetchedAt time.Time) error {
	if s.snaps[vin] == nil {
		s.snaps[vin] = map[string]Snapshot{}
	}
	s.snaps[vin][source] = Snapshot{Documents: docs, FetchedAt: fetchedAt}
	s.puts++
	return nil
}

func doc(id, source string, date string) domain.Document {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err) // test data bug, fail loudly
	}
	return domain.Document{ID: id, Title: id, Type: "x", Date: d, Source: source}
}

func TestAggregator_Documents(t *testing.T) {
	tests := []struct {
		name        string
		sources     []domain.Source
		timeout     time.Duration
		wantLen     int
		wantPartial bool
		wantErrs    int
	}{
		{
			name: "both ok, sorted newest first",
			sources: []domain.Source{
				// older doc first on purpose: a wrong sort direction must fail this case
				fakeSource{name: "A", docs: []domain.Document{doc("A-1", "A", "2024-07-01")}},
				fakeSource{name: "B", docs: []domain.Document{doc("B-1", "B", "2024-09-15")}},
			},
			wantLen: 2, wantPartial: false, wantErrs: 0,
		},
		{
			name: "one fails",
			sources: []domain.Source{
				fakeSource{name: "A", docs: []domain.Document{doc("A-1", "A", "2024-07-01")}},
				fakeSource{name: "B", err: errors.New("boom")},
			},
			wantLen: 1, wantPartial: true, wantErrs: 1,
		},
		{
			name: "both fail",
			sources: []domain.Source{
				fakeSource{name: "A", err: errors.New("boom")},
				fakeSource{name: "B", err: errors.New("boom")},
			},
			wantLen: 0, wantPartial: true, wantErrs: 2,
		},
		{
			name: "one empty is not partial",
			sources: []domain.Source{
				fakeSource{name: "A", docs: []domain.Document{doc("A-1", "A", "2024-07-01")}},
				fakeSource{name: "B", docs: []domain.Document{}},
			},
			wantLen: 1, wantPartial: false, wantErrs: 0,
		},
		{
			name: "slow source times out",
			sources: []domain.Source{
				fakeSource{name: "A", docs: []domain.Document{doc("A-1", "A", "2024-07-01")}},
				fakeSource{name: "B", delay: 2 * time.Second},
			},
			timeout: 50 * time.Millisecond,
			wantLen: 1, wantPartial: true, wantErrs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			agg := New(tt.sources, timeout, newFakeStore(), 15*time.Minute)
			got := agg.Documents(context.Background(), "1HGCM82633A004352")

			if got.VIN != "1HGCM82633A004352" {
				t.Errorf("VIN = %q", got.VIN)
			}
			if len(got.Documents) != tt.wantLen {
				t.Fatalf("len(Documents) = %d; want %d", len(got.Documents), tt.wantLen)
			}
			if got.Partial != tt.wantPartial {
				t.Errorf("Partial = %v; want %v", got.Partial, tt.wantPartial)
			}
			if len(got.Errors) != tt.wantErrs {
				t.Errorf("len(Errors) = %d; want %d", len(got.Errors), tt.wantErrs)
			}

			switch tt.name {
			case "both ok, sorted newest first":
				if got.Documents[0].ID != "B-1" {
					t.Errorf("Documents[0].ID = %q; want B-1 (newest first)", got.Documents[0].ID)
				}
			case "one fails":
				if got.Errors[0].Source != "B" {
					t.Errorf("Error source = %q; want B", got.Errors[0].Source)
				}

			case "both fail":
				if got.Documents == nil {
					t.Errorf("Documents source want []")
				}
			case "slow source times out":
				if !strings.Contains(got.Errors[0].Reason, "deadline") {
					t.Errorf("Error reason %v want deadline", got.Errors[0].Reason)
				}

			}

		})
	}
}

func TestAggregator_RunsSourcesInParallel(t *testing.T) {
	sources := []domain.Source{
		fakeSource{name: "A", delay: 100 * time.Millisecond},
		fakeSource{name: "B", delay: 100 * time.Millisecond},
	}
	agg := New(sources, time.Second, newFakeStore(), 15*time.Minute)

	start := time.Now()
	agg.Documents(context.Background(), "X")
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Errorf("took %v; want < 150ms (sources must run in parallel)", elapsed)
	}
}

// Cache behaviour: data flow steps 3, 5 and 7 of the design doc.
func TestAggregator_Cache(t *testing.T) {
	const vin = "1HGCM82633A004352"
	const ttl = 15 * time.Minute
	fresh := time.Now().Add(-1 * time.Minute)  // inside TTL
	expired := time.Now().Add(-1 * time.Hour) // outside TTL

	t.Run("fresh snapshot: source not called, no Put", func(t *testing.T) {
		store := newFakeStore()
		store.Put(context.Background(), vin, "A", []domain.Document{doc("CACHED", "A", "2024-07-01")}, fresh)
		var calls int32
		src := fakeSource{name: "A", calls: &calls, docs: []domain.Document{doc("LIVE", "A", "2024-07-02")}}

		got := New([]domain.Source{src}, time.Second, store, ttl).Documents(context.Background(), vin)

		if calls != 0 {
			t.Errorf("source called %d times; want 0 (cache hit)", calls)
		}
		if len(got.Documents) != 1 || got.Documents[0].ID != "CACHED" {
			t.Errorf("Documents = %v; want the cached doc", got.Documents)
		}
		if store.puts != 1 {
			t.Errorf("Put called %d times; want 1 (only the test's own seed)", store.puts)
		}
		if got.Stale || got.Partial {
			t.Errorf("Stale=%v Partial=%v; want both false on a fresh cache hit", got.Stale, got.Partial)
		}
	})

	t.Run("expired snapshot: source called, result written back", func(t *testing.T) {
		store := newFakeStore()
		store.Put(context.Background(), vin, "A", []domain.Document{doc("OLD", "A", "2024-07-01")}, expired)
		var calls int32
		src := fakeSource{name: "A", calls: &calls, docs: []domain.Document{doc("LIVE", "A", "2024-07-02")}}

		got := New([]domain.Source{src}, time.Second, store, ttl).Documents(context.Background(), vin)

		if calls != 1 {
			t.Errorf("source called %d times; want 1 (cache miss)", calls)
		}
		if len(got.Documents) != 1 || got.Documents[0].ID != "LIVE" {
			t.Errorf("Documents = %v; want the live doc", got.Documents)
		}
		if store.puts != 2 {
			t.Errorf("Put called %d times; want 2 (seed + write-back)", store.puts)
		}
		if snap := store.snaps[vin]["A"]; len(snap.Documents) != 1 || snap.Documents[0].ID != "LIVE" {
			t.Errorf("stored snapshot = %v; want the live doc", snap.Documents)
		}
		if got.Stale {
			t.Errorf("Stale = true; want false (fresh data was fetched)")
		}
	})

	t.Run("source fails, expired snapshot exists: stale fallback", func(t *testing.T) {
		store := newFakeStore()
		store.Put(context.Background(), vin, "A", []domain.Document{doc("OLD", "A", "2024-07-01")}, expired)
		src := fakeSource{name: "A", err: errors.New("boom")}

		got := New([]domain.Source{src}, time.Second, store, ttl).Documents(context.Background(), vin)

		if !got.Partial || !got.Stale {
			t.Errorf("Partial=%v Stale=%v; want both true", got.Partial, got.Stale)
		}
		if len(got.Documents) != 1 || got.Documents[0].ID != "OLD" {
			t.Errorf("Documents = %v; want the old snapshot", got.Documents)
		}
		if store.puts != 1 {
			t.Errorf("Put called %d times; want 1 (a failed fetch must not overwrite the snapshot)", store.puts)
		}
	})

	t.Run("source fails, no snapshot: partial only", func(t *testing.T) {
		store := newFakeStore()
		src := fakeSource{name: "A", err: errors.New("boom")}

		got := New([]domain.Source{src}, time.Second, store, ttl).Documents(context.Background(), vin)

		if !got.Partial || got.Stale {
			t.Errorf("Partial=%v Stale=%v; want Partial=true Stale=false", got.Partial, got.Stale)
		}
		if len(got.Documents) != 0 || got.Documents == nil {
			t.Errorf("Documents = %v; want empty non-nil slice", got.Documents)
		}
		if store.puts != 0 {
			t.Errorf("Put called %d times; want 0", store.puts)
		}
	})
}
