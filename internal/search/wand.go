package search

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
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

func searchWAND(idx *indexfile.Index, query string, k int) ([]result, error) {
	if k <= 0 {
		if _, err := prepareQuery(query); err != nil {
			return nil, err
		}
		return nil, nil
	}

	plan, err := buildDiskQueryPlan(idx, query)
	if err != nil {
		return nil, err
	}
	return executeWAND(idx, plan, k)
}

func executeWAND(idx *indexfile.Index, plan diskQueryPlan, k int) ([]result, error) {
	collector := newTopK(k)
	for {
		cursors := currentWANDCursors(plan.terms)
		if len(cursors) == 0 {
			break
		}

		documentID := cursors[0].documentID
		for _, cursor := range cursors[1:] {
			documentID = min(documentID, cursor.documentID)
		}

		if threshold, active := collector.threshold(); active {
			pivot, found := selectWANDPivot(&plan, cursors, threshold)
			if !found {
				break
			}
			if pivot.documentID != documentID {
				cursorToAdvance := pivot.preceding[0]
				for _, cursor := range pivot.preceding[1:] {
					if cursor.documentID == pivot.documentID {
						break
					}
					if plan.terms[cursor.termIndex].idf > plan.terms[cursorToAdvance.termIndex].idf {
						cursorToAdvance = cursor
					}
				}

				term := &plan.terms[cursorToAdvance.termIndex]
				if _, err := term.cursor.Advance(pivot.documentID); err != nil {
					return nil, fmt.Errorf("advance %q postings to document %d: %w", term.term, pivot.documentID, err)
				}
				continue
			}
		}

		documentLength := idx.DocumentLength(documentID)
		var score float64
		for termIndex := range plan.terms {
			term := &plan.terms[termIndex]
			posting, current := term.cursor.Current()
			if !current || posting.DocumentID != documentID {
				continue
			}
			score += bm25TermScore(
				term.idf,
				posting.Frequency,
				documentLength,
				plan.averageDocumentLength,
			)
			if _, err := term.cursor.Next(); err != nil {
				return nil, fmt.Errorf("advance %q postings: %w", term.term, err)
			}
		}
		collector.add(result{DocumentID: documentID, Score: score})
	}

	results := collector.finish()
	if err := resolveExternalIDs(idx, results); err != nil {
		return nil, err
	}
	return results, nil
}

func currentWANDCursors(terms []diskQueryTerm) []wandCursor {
	cursors := make([]wandCursor, 0, len(terms))
	for termIndex, term := range terms {
		posting, current := term.cursor.Current()
		if current {
			cursors = append(cursors, wandCursor{termIndex: termIndex, documentID: posting.DocumentID})
		}
	}
	return cursors
}
