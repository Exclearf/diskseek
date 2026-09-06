package search

import (
	"cmp"
	"container/heap"
	"fmt"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

type result struct {
	DocumentID index.DocumentID
	ExternalID string
	Score      float64
}

func compareResults(left, right result) int {
	if left.Score > right.Score {
		return -1
	}
	if left.Score < right.Score {
		return 1
	}
	return cmp.Compare(left.DocumentID, right.DocumentID)
}

func compareWorstResults(left, right result) int {
	return compareResults(right, left)
}

type topK struct {
	limit int
	items resultHeap
}

func newTopK(limit int) topK {
	return topK{limit: limit}
}

func (t *topK) add(candidate result) {
	if t.limit <= 0 {
		return
	}
	if len(t.items) < t.limit {
		heap.Push(&t.items, candidate)
		return
	}
	if compareResults(candidate, t.items[0]) >= 0 {
		return
	}

	t.items[0] = candidate
	heap.Fix(&t.items, 0)
}

func (t *topK) threshold() (float64, bool) {
	if t.limit <= 0 || len(t.items) < t.limit {
		return 0, false
	}
	return t.items[0].Score, true
}

func (t *topK) finish() []result {
	results := []result(t.items)
	slices.SortFunc(results, compareResults)
	return results
}

func resolveExternalIDs(idx *indexfile.Index, results []result) error {
	for position := range results {
		externalID, err := idx.ExternalID(results[position].DocumentID)
		if err != nil {
			return fmt.Errorf("resolve document %d: %w", results[position].DocumentID, err)
		}
		results[position].ExternalID = externalID
	}
	return nil
}

type resultHeap []result

func (h resultHeap) Len() int {
	return len(h)
}

func (h resultHeap) Less(left, right int) bool {
	return compareWorstResults(h[left], h[right]) < 0
}

func (h resultHeap) Swap(left, right int) {
	h[left], h[right] = h[right], h[left]
}

func (h *resultHeap) Push(value any) {
	*h = append(*h, value.(result))
}

func (h *resultHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}
