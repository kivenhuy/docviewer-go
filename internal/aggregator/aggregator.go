package aggregator

import (
	"context"
	"slices"
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

type Aggregator struct {
	sources []domain.Source
	timeout time.Duration
}

func New(sources []domain.Source, timeout time.Duration) *Aggregator {
	return &Aggregator{
		sources: sources,
		timeout: timeout,
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

	for _, src := range a.sources {
		rc, cancel := context.WithTimeout(ctx, a.timeout)
		docs, err := src.Fetch(rc, vin)
		cancel()
		if err != nil {
			data.Partial = true
			data.Errors = append(data.Errors, SourceError{Source: src.Name(), Reason: err.Error()})
			continue
		}
		data.Documents = append(data.Documents, docs...)

	}
	slices.SortFunc(data.Documents, func(x, y domain.Document) int {
		return y.Date.Compare(x.Date)
	})

	return data
}
