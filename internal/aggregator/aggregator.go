package aggregator

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
)

type SourceError struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type Result struct {
	VIN       string            `json:"vin"`
	Documents []domain.Document `json:"documents"`
	Partial   bool              `json:"partial"`
	Stale     bool              `json:"stale"`
	Errors    []SourceError     `json:"errors,omitempty"`
}

type fetchResult struct {
	docs      []domain.Document
	fromCache bool
	err       error
}

type Aggregator struct {
	sources []domain.Source
	timeout time.Duration
	store   SnapshotStore
	ttl     time.Duration
	logger  CallLogger
}

func New(sources []domain.Source, timeout time.Duration, store SnapshotStore, ttl time.Duration, logger CallLogger) *Aggregator {
	return &Aggregator{
		sources: sources,
		timeout: timeout,
		store:   store,
		ttl:     ttl,
		logger:  logger,
	}
}

func (a *Aggregator) Documents(ctx context.Context, vin string) Result {

	snaps, err := a.store.Get(ctx, vin)
	if err != nil {
		slog.Warn("snapshot store get", "vin", vin, "err", err)
	}

	data := Result{
		VIN:       vin,
		Documents: []domain.Document{},
		Partial:   false,
		Stale:     false,
		Errors:    []SourceError{},
	}
	result := make([]fetchResult, len(a.sources))
	var wg sync.WaitGroup
	for i, src := range a.sources {
		wg.Add(1)
		go func() {
			defer wg.Done()

			snap, ok := snaps[src.Name()]
			if ok && time.Since(snap.FetchedAt) < a.ttl {
				result[i] = fetchResult{docs: snap.Documents, fromCache: true}
				return
			}
			rc, cancel := context.WithTimeout(ctx, a.timeout)
			defer cancel()
			start := time.Now()
			docs, err := src.Fetch(rc, vin)
			var outcome string
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				outcome = "timeout"
			case err != nil:
				outcome = "error"
			case len(docs) == 0:
				outcome = "empty"
			default:
				outcome = "ok"
			}
			if err := a.logger.LogCall(ctx, CallLog{
				VIN: vin, Source: src.Name(), Outcome: outcome,
				Duration: time.Since(start), Err: err,
			}); err != nil {
				slog.Warn("call log", "vin", vin, "source", src.Name(), "err", err)
			}

			result[i] = fetchResult{docs: docs, err: err}
		}()

	}
	wg.Wait()
	for i, src := range a.sources {
		r := result[i]
		if r.err == nil && !r.fromCache {
			if err := a.store.Put(ctx, vin, src.Name(), r.docs, time.Now()); err != nil {
				slog.Warn("snapshot store put", "vin", vin, "source", src.Name(), "err", err)
			}
		}

		if r.err != nil {
			data.Partial = true
			data.Errors = append(data.Errors, SourceError{Source: src.Name(), Reason: r.err.Error()})
			if snap, ok := snaps[src.Name()]; ok {
				data.Documents = append(data.Documents, snap.Documents...)
				data.Stale = true
			}
			continue
		}
		data.Documents = append(data.Documents, r.docs...)
	}
	slices.SortFunc(data.Documents, func(x, y domain.Document) int {
		return y.Date.Compare(x.Date)
	})

	return data
}
