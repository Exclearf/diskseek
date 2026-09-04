package search

import (
	"cmp"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
)

type result struct {
	DocumentID index.DocumentID
	Score      float64
}

func referenceSearch(idx *index.Index, query string, k int) ([]result, error) {
	terms, err := prepareQuery(query)
	if err != nil {
		return nil, err
	}
	if k <= 0 || len(terms) == 0 || idx.DocumentsWithTerms == 0 {
		return nil, nil
	}

	averageDocumentLength := float64(idx.TotalLength) / float64(idx.DocumentsWithTerms)
	scores := make(map[index.DocumentID]float64)

	for _, term := range terms {
		postings := idx.Postings[term]
		if len(postings) == 0 {
			continue
		}

		idf := bm25IDF(idx.DocumentsWithTerms, uint64(len(postings)))
		for _, posting := range postings {
			documentLength := idx.Documents[posting.DocumentID].Length
			scores[posting.DocumentID] += bm25TermScore(
				idf,
				posting.Frequency,
				documentLength,
				averageDocumentLength,
			)
		}
	}

	results := make([]result, 0, len(scores))
	for documentID, score := range scores {
		results = append(results, result{DocumentID: documentID, Score: score})
	}
	slices.SortFunc(results, compareResults)

	if len(results) > k {
		results = results[:k]
	}
	return results, nil
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
