package search

import (
	"cmp"
	"context"
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

type wandStats struct {
	NextCalls        uint64
	AdvanceCalls     uint64
	BlockHeadersRead uint64
	BlocksSkipped    uint64
	BlocksDecoded    uint64
	PostingsDecoded  uint64
	BytesRequested   uint64
	CandidatesScored uint64
	PivotSelections  uint64
	ThresholdChanges uint64
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

func searchWAND(ctx context.Context, idx *indexfile.Index, query string, k int) ([]result, wandStats, error) {
	if k <= 0 {
		if err := ctx.Err(); err != nil {
			return nil, wandStats{}, err
		}
		if _, err := prepareQuery(query); err != nil {
			return nil, wandStats{}, err
		}
		if err := ctx.Err(); err != nil {
			return nil, wandStats{}, err
		}
		return nil, wandStats{}, nil
	}

	plan, err := buildDiskQueryPlan(ctx, idx, query)
	if err != nil {
		return nil, wandStats{}, err
	}
	return executeWAND(ctx, idx, plan, k)
}

func executeWAND(ctx context.Context, idx *indexfile.Index, plan diskQueryPlan, k int) ([]result, wandStats, error) {
	collector := newTopK(k)
	var stats wandStats
	for {
		if err := ctx.Err(); err != nil {
			return nil, wandStats{}, err
		}
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
			stats.PivotSelections++
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
				if _, err := term.cursor.AdvanceContext(ctx, pivot.documentID); err != nil {
					return nil, wandStats{}, fmt.Errorf("advance %q postings to document %d: %w", term.term, pivot.documentID, err)
				}
				continue
			}
		}

		documentLength := idx.DocumentLength(documentID)
		var score float64
		for termIndex := range plan.terms {
			if err := ctx.Err(); err != nil {
				return nil, wandStats{}, err
			}
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
			if _, err := term.cursor.NextContext(ctx); err != nil {
				return nil, wandStats{}, fmt.Errorf("advance %q postings: %w", term.term, err)
			}
		}
		previousThreshold, previouslyActive := collector.threshold()
		collector.add(result{DocumentID: documentID, Score: score})
		stats.CandidatesScored++
		threshold, active := collector.threshold()
		if active && (!previouslyActive || threshold != previousThreshold) {
			stats.ThresholdChanges++
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, wandStats{}, err
	}
	results := collector.finish()
	if err := resolveExternalIDs(ctx, idx, results); err != nil {
		return nil, wandStats{}, err
	}
	for _, term := range plan.terms {
		cursorStats := term.cursor.Stats()
		stats.NextCalls += cursorStats.NextCalls
		stats.AdvanceCalls += cursorStats.AdvanceCalls
		stats.BlockHeadersRead += cursorStats.BlockHeadersRead
		stats.BlocksSkipped += cursorStats.BlocksSkipped
		stats.BlocksDecoded += cursorStats.BlocksDecoded
		stats.PostingsDecoded += cursorStats.PostingsDecoded
		stats.BytesRequested += cursorStats.BytesRequested
	}
	return results, stats, nil
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
