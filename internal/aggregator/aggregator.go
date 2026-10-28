package aggregator

import (
	"context"
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
	docs []domain.Document
	err  error
}

type Aggregator struct {
	sources []domain.Source
	timeout time.Duration
	store   SnapshotStore
	ttl     time.Duration
}

func New(sources []domain.Source, timeout time.Duration, store SnapshotStore, ttl time.Duration) *Aggregator {
	return &Aggregator{
		sources: sources,
		timeout: timeout,
		store:   store,
		ttl:     ttl,
	}
}

func (a *Aggregator) Documents(ctx context.Context, vin string) Result {
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
			rc, cancel := context.WithTimeout(ctx, a.timeout)
			docs, err := src.Fetch(rc, vin)
			result[i] = fetchResult{docs: docs, err: err}
			cancel()
		}()

	}
	wg.Wait()
	for i, src := range a.sources {
		r := result[i]

		if r.err != nil {
			data.Partial = true
			data.Errors = append(data.Errors, SourceError{Source: src.Name(), Reason: r.err.Error()})
			continue
		}
		data.Documents = append(data.Documents, r.docs...)
	}
	slices.SortFunc(data.Documents, func(x, y domain.Document) int {
		return y.Date.Compare(x.Date)
	})

	return data
}
