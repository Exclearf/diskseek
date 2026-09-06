package search

import (
	"context"
	"fmt"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

type daatStats struct {
	PostingsDecoded  uint64
	NextCalls        uint64
	AdvanceCalls     uint64
	CandidatesScored uint64
	BytesRequested   uint64
}

func searchDAAT(ctx context.Context, idx *indexfile.Index, query string, k int) ([]result, daatStats, error) {
	if k <= 0 {
		if err := ctx.Err(); err != nil {
			return nil, daatStats{}, err
		}
		if _, err := prepareQuery(query); err != nil {
			return nil, daatStats{}, err
		}
		if err := ctx.Err(); err != nil {
			return nil, daatStats{}, err
		}
		return nil, daatStats{}, nil
	}

	plan, err := buildDiskQueryPlan(ctx, idx, query)
	if err != nil {
		return nil, daatStats{}, err
	}
	return executeDAAT(ctx, idx, plan, k)
}

func executeDAAT(ctx context.Context, idx *indexfile.Index, plan diskQueryPlan, k int) ([]result, daatStats, error) {
	collector := newTopK(k)
	var candidatesScored uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, daatStats{}, err
		}
		documentID, found := nextCandidate(plan.terms)
		if !found {
			break
		}

		documentLength := idx.DocumentLength(documentID)
		var score float64
		for termIndex := range plan.terms {
			if err := ctx.Err(); err != nil {
				return nil, daatStats{}, err
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
				return nil, daatStats{}, fmt.Errorf("advance %q postings: %w", term.term, err)
			}
		}
		collector.add(result{DocumentID: documentID, Score: score})
		candidatesScored++
	}

	if err := ctx.Err(); err != nil {
		return nil, daatStats{}, err
	}
	results := collector.finish()
	if err := resolveExternalIDs(ctx, idx, results); err != nil {
		return nil, daatStats{}, err
	}

	stats := daatStats{CandidatesScored: candidatesScored}
	for _, term := range plan.terms {
		cursorStats := term.cursor.Stats()
		stats.PostingsDecoded += cursorStats.PostingsDecoded
		stats.NextCalls += cursorStats.NextCalls
		stats.AdvanceCalls += cursorStats.AdvanceCalls
		stats.BytesRequested += cursorStats.BytesRequested
	}
	return results, stats, nil
}

func nextCandidate(terms []diskQueryTerm) (index.DocumentID, bool) {
	var documentID index.DocumentID
	found := false
	for _, term := range terms {
		posting, current := term.cursor.Current()
		if current && (!found || posting.DocumentID < documentID) {
			documentID = posting.DocumentID
			found = true
		}
	}
	return documentID, found
}
