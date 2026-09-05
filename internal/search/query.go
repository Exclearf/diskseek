package search

import (
	"fmt"
	"slices"

	"github.com/Exclearf/diskseek/internal/analyzer"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

type diskQueryPlan struct {
	terms                 []diskQueryTerm
	averageDocumentLength float64
}

type diskQueryTerm struct {
	term       string
	idf        float64
	upperBound float64
	cursor     *indexfile.Cursor
}

func prepareQuery(query string) ([]string, error) {
	terms, err := analyzer.Analyze(query)
	if err != nil {
		return nil, err
	}

	slices.Sort(terms)
	return slices.Compact(terms), nil
}

func buildDiskQueryPlan(idx *indexfile.Index, query string) (diskQueryPlan, error) {
	terms, err := prepareQuery(query)
	if err != nil {
		return diskQueryPlan{}, err
	}

	plan := diskQueryPlan{averageDocumentLength: idx.AverageDocumentLength()}
	for _, term := range terms {
		cursor, found, err := idx.Postings(term)
		if err != nil {
			return diskQueryPlan{}, fmt.Errorf("open %q postings: %w", term, err)
		}
		if !found {
			continue
		}
		idf := bm25IDF(idx.DocumentsWithTerms(), cursor.DocumentFrequency())
		plan.terms = append(plan.terms, diskQueryTerm{
			term:       term,
			idf:        idf,
			upperBound: bm25TermUpperBound(idf, cursor.MaxTermFrequency(), plan.averageDocumentLength),
			cursor:     cursor,
		})
	}
	return plan, nil
}
