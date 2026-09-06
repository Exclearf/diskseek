package search

import (
	"context"
	"errors"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

type Executor uint8

const (
	ExecutorWAND Executor = 0
	ExecutorDAAT Executor = 1
)

type MeasuredResult struct {
	DocumentID index.DocumentID
	ExternalID string
	Score      float64
}

type QueryStats struct {
	QueryTerms            uint64
	MatchedTerms          uint64
	NextCalls             uint64
	AdvanceCalls          uint64
	BlockHeadersRead      uint64
	BlocksSkipped         uint64
	BlocksDecoded         uint64
	PostingsDecoded       uint64
	LogicalBytesRequested uint64
	CandidatesScored      uint64
	PivotSelections       uint64
	ThresholdChanges      uint64
}

func SearchWithStats(
	ctx context.Context,
	idx *indexfile.Index,
	query string,
	limit int,
	executor Executor,
) ([]MeasuredResult, QueryStats, error) {
	var (
		ranked []result
		stats  QueryStats
		err    error
	)
	switch executor {
	case ExecutorWAND:
		ranked, stats, err = searchWAND(ctx, idx, query, limit)
	case ExecutorDAAT:
		ranked, stats, err = searchDAAT(ctx, idx, query, limit)
	default:
		return nil, QueryStats{}, errors.New("unsupported search executor")
	}
	if err != nil {
		return nil, QueryStats{}, err
	}

	results := make([]MeasuredResult, len(ranked))
	for position, item := range ranked {
		results[position] = MeasuredResult{
			DocumentID: item.DocumentID,
			ExternalID: item.ExternalID,
			Score:      item.Score,
		}
	}
	return results, stats, nil
}
