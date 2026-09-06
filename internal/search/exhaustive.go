package search

import (
	"context"
	"fmt"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func searchDAAT(ctx context.Context, idx *indexfile.Index, query string, k int) ([]result, QueryStats, error) {
	if k <= 0 {
		if err := ctx.Err(); err != nil {
			return nil, QueryStats{}, err
		}
		if _, err := prepareQuery(query); err != nil {
			return nil, QueryStats{}, err
		}
		if err := ctx.Err(); err != nil {
			return nil, QueryStats{}, err
		}
		return nil, QueryStats{}, nil
	}

	plan, err := buildDiskQueryPlan(ctx, idx, query)
	if err != nil {
		return nil, QueryStats{}, err
	}
	return executeDAAT(ctx, idx, plan, k)
}

func executeDAAT(ctx context.Context, idx *indexfile.Index, plan diskQueryPlan, k int) ([]result, QueryStats, error) {
	collector := newTopK(k)
	var candidatesScored uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, QueryStats{}, err
		}
		documentID, found := nextCandidate(plan.terms)
		if !found {
			break
		}

		documentLength := idx.DocumentLength(documentID)
		var score float64
		for termIndex := range plan.terms {
			if err := ctx.Err(); err != nil {
				return nil, QueryStats{}, err
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
				return nil, QueryStats{}, fmt.Errorf("advance %q postings: %w", term.term, err)
			}
		}
		collector.add(result{DocumentID: documentID, Score: score})
		candidatesScored++
	}

	if err := ctx.Err(); err != nil {
		return nil, QueryStats{}, err
	}
	results := collector.finish()
	if err := resolveExternalIDs(ctx, idx, results); err != nil {
		return nil, QueryStats{}, err
	}

	stats := QueryStats{
		QueryTerms:       plan.queryTerms,
		MatchedTerms:     uint64(len(plan.terms)),
		CandidatesScored: candidatesScored,
	}
	for _, term := range plan.terms {
		cursorStats := term.cursor.Stats()
		stats.NextCalls += cursorStats.NextCalls
		stats.AdvanceCalls += cursorStats.AdvanceCalls
		stats.BlockHeadersRead += cursorStats.BlockHeadersRead
		stats.BlocksSkipped += cursorStats.BlocksSkipped
		stats.BlocksDecoded += cursorStats.BlocksDecoded
		stats.PostingsDecoded += cursorStats.PostingsDecoded
		stats.LogicalBytesRequested += cursorStats.BytesRequested
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
