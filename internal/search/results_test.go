package search

import (
	"cmp"
	"testing"
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
