package aggregator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kivenhuy/docviewer-go/internal/domain"
)

// fakeSource satisfies domain.Source without any HTTP.
// This is the payoff of the Source interface: the aggregator can be tested in isolation.
type fakeSource struct {
	name string
	docs []domain.Document
	err  error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(ctx context.Context, vin string) ([]domain.Document, error) {
	return f.docs, f.err
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg := New(tt.sources, time.Second)
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
			}
		})
	}
}
