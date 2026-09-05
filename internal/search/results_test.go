package search

import (
	"cmp"
	"fmt"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestResultOrder(t *testing.T) {
	tests := []struct {
		name      string
		left      result
		right     result
		wantOrder int
	}{
		{
			name:      "higher score first",
			left:      result{DocumentID: 9, Score: 2},
			right:     result{DocumentID: 1, Score: 1},
			wantOrder: -1,
		},
		{
			name:      "lower document ID first on tie",
			left:      result{DocumentID: 1, Score: 2},
			right:     result{DocumentID: 9, Score: 2},
			wantOrder: -1,
		},
		{
			name:  "equal result",
			left:  result{DocumentID: 1, Score: 2},
			right: result{DocumentID: 1, Score: 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cmp.Compare(compareResults(test.left, test.right), 0); got != test.wantOrder {
				t.Fatalf("compareResults() order = %d, want %d", got, test.wantOrder)
			}
			if got := cmp.Compare(compareWorstResults(test.left, test.right), 0); got != -test.wantOrder {
				t.Fatalf("compareWorstResults() order = %d, want %d", got, -test.wantOrder)
			}
		})
	}
}

func TestTopKMatchesFullSort(t *testing.T) {
	const candidateCount = 64
	candidates := make([]result, candidateCount)
	for i := range candidates {
		candidates[i] = result{
			DocumentID: index.DocumentID((i * 37) % candidateCount),
			Score:      float64((i * 5) % 7),
		}
	}

	for _, k := range []int{0, 1, 8, candidateCount, candidateCount + 10} {
		t.Run(fmt.Sprintf("k=%d", k), func(t *testing.T) {
			collector := newTopK(k)
			for _, candidate := range candidates {
				collector.add(candidate)
				if len(collector.items) > k {
					t.Fatalf("collector retained %d results with k = %d", len(collector.items), k)
				}
			}

			want := slices.Clone(candidates)
			slices.SortFunc(want, compareResults)
			if len(want) > k {
				want = want[:k]
			}

			got := collector.finish()
			if !slices.Equal(got, want) {
				t.Fatalf("top-k results = %+v, want %+v", got, want)
			}
		})
	}
}

func TestTopKThresholdRequiresFullCollector(t *testing.T) {
	collector := newTopK(2)
	collector.add(result{DocumentID: 0, Score: 2})
	if _, active := collector.threshold(); active {
		t.Fatal("threshold active before collector is full")
	}

	collector.add(result{DocumentID: 1, Score: 1})
	threshold, active := collector.threshold()
	if !active || threshold != 1 {
		t.Fatalf("threshold = (%v, %t), want (1, true)", threshold, active)
	}
}
