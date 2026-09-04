package search

import (
	"slices"

	"github.com/Exclearf/diskseek/internal/analyzer"
)

func prepareQuery(query string) ([]string, error) {
	terms, err := analyzer.Analyze(query)
	if err != nil {
		return nil, err
	}

	slices.Sort(terms)
	return slices.Compact(terms), nil
}
