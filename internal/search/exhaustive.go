package search

import (
	"fmt"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func searchDAAT(idx *indexfile.Index, query string, k int) ([]result, error) {
	if k <= 0 {
		return nil, nil
	}

	plan, err := buildDiskQueryPlan(idx, query)
	if err != nil {
		return nil, err
	}
	return executeDAAT(idx, plan, k)
}

func executeDAAT(idx *indexfile.Index, plan diskQueryPlan, k int) ([]result, error) {
	collector := newTopK(k)
	for {
		documentID, found := nextCandidate(plan.terms)
		if !found {
			break
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
	for position := range results {
		externalID, err := idx.ExternalID(results[position].DocumentID)
		if err != nil {
			return nil, fmt.Errorf("resolve document %d: %w", results[position].DocumentID, err)
		}
		results[position].ExternalID = externalID
	}
	return results, nil
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
