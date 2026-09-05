package search

import (
	"math"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestSelectWANDPivot(t *testing.T) {
	tests := []struct {
		name          string
		bounds        []float64
		cursors       []wandCursor
		threshold     float64
		wantDocument  index.DocumentID
		wantPreceding []wandCursor
		wantFound     bool
	}{
		{
			name:      "complete bound equals threshold",
			bounds:    []float64{0.5, 0.5},
			cursors:   []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold: 1,
		},
		{
			name:      "complete bound one ULP below threshold",
			bounds:    []float64{0.5, 0.5},
			cursors:   []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold: math.Nextafter(1, math.Inf(1)),
		},
		{
			name:          "complete bound one ULP above threshold",
			bounds:        []float64{math.Nextafter(1, math.Inf(1))},
			cursors:       []wandCursor{{termIndex: 0, documentID: 2}},
			threshold:     1,
			wantDocument:  2,
			wantPreceding: []wandCursor{},
			wantFound:     true,
		},
		{
			name:          "equal prefix followed by larger prefix",
			bounds:        []float64{0.25, 0.75, 0.5},
			cursors:       []wandCursor{{termIndex: 2, documentID: 8}, {termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold:     1,
			wantDocument:  8,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 2}, {termIndex: 1, documentID: 5}},
			wantFound:     true,
		},
		{
			name:          "pivot not aligned with first cursor",
			bounds:        []float64{0.4, 0.8, 0.7},
			cursors:       []wandCursor{{termIndex: 2, documentID: 5}, {termIndex: 0, documentID: 2}, {termIndex: 1, documentID: 5}},
			threshold:     1,
			wantDocument:  5,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 2}},
			wantFound:     true,
		},
		{
			name:          "equal document IDs use canonical term order",
			bounds:        []float64{0.4, 0.6},
			cursors:       []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 5}},
			threshold:     0.5,
			wantDocument:  5,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 5}},
			wantFound:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := diskQueryPlan{terms: make([]diskQueryTerm, len(test.bounds))}
			for termIndex, bound := range test.bounds {
				plan.terms[termIndex].upperBound = bound
			}

			pivot, found := selectWANDPivot(&plan, slices.Clone(test.cursors), test.threshold)
			if found != test.wantFound {
				t.Fatalf("pivot found = %t, want %t", found, test.wantFound)
			}
			if !found {
				return
			}
			if pivot.documentID != test.wantDocument || !slices.Equal(pivot.preceding, test.wantPreceding) {
				t.Fatalf("pivot = %+v, want document %d preceding %+v", pivot, test.wantDocument, test.wantPreceding)
			}
		})
	}
}
