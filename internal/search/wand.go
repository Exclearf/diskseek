package search

import (
	"cmp"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
)

type wandCursor struct {
	termIndex  int
	documentID index.DocumentID
}

type wandPivot struct {
	documentID index.DocumentID
	preceding  []wandCursor
}

func selectWANDPivot(plan *diskQueryPlan, cursors []wandCursor, threshold float64) (wandPivot, bool) {
	slices.SortFunc(cursors, func(left, right wandCursor) int {
		if order := cmp.Compare(left.documentID, right.documentID); order != 0 {
			return order
		}
		return cmp.Compare(left.termIndex, right.termIndex)
	})

	selected := make([]bool, len(plan.terms))
	for cursorIndex, cursor := range cursors {
		selected[cursor.termIndex] = true
		if plan.selectedUpperBound(selected) > threshold {
			return wandPivot{
				documentID: cursor.documentID,
				preceding:  cursors[:cursorIndex],
			}, true
		}
	}
	return wandPivot{}, false
}
