package search

import (
	"math"
	"os"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func TestReferenceSearch(t *testing.T) {
	input, err := os.Open("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	idx, err := index.Build(corpus.NewTSVReader(input))
	if err != nil {
		t.Fatal(err)
	}

	const tolerance = 1e-12
	tests := []struct {
		name  string
		query string
		k     int
		want  []result
	}{
		{
			name:  "duplicate and unknown terms",
			query: "missing FAST fast",
			k:     10,
			want:  []result{{DocumentID: 0, Score: 1.203972804325936}},
		},
		{
			name:  "tie and k beyond matches",
			query: "search index",
			k:     10,
			want: []result{
				{DocumentID: 0, Score: 1.386294361119891},
				{DocumentID: 1, Score: 1.386294361119891},
			},
		},
		{
			name:  "truncation",
			query: "search fast",
			k:     1,
			want:  []result{{DocumentID: 0, Score: 1.897119984885881}},
		},
		{name: "zero k", query: "fast", k: 0},
		{name: "empty query", query: "---", k: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := referenceSearch(&idx, test.query, test.k)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("referenceSearch() returned %d results, want %d", len(got), len(test.want))
			}
			for i := range got {
				if got[i].DocumentID != test.want[i].DocumentID ||
					math.IsNaN(got[i].Score) || math.Abs(got[i].Score-test.want[i].Score) > tolerance {
					t.Fatalf("referenceSearch() result %d = %+v, want %+v", i, got[i], test.want[i])
				}
			}
		})
	}

	first, err := referenceSearch(&idx, "search fast", 10)
	if err != nil {
		t.Fatal(err)
	}
	second, err := referenceSearch(&idx, "search fast", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !equalResultBits(first, second) {
		t.Fatalf("referenceSearch() = %+v, repeated call = %+v", first, second)
	}
}

func equalResultBits(left, right []result) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].DocumentID != right[i].DocumentID ||
			math.Float64bits(left[i].Score) != math.Float64bits(right[i].Score) {
			return false
		}
	}
	return true
}
