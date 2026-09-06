package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
)

type queryOptions struct {
	runID        string
	repetition   int
	executor     search.Executor
	executorName string
	limit        int
	warmup       int
}

type queryObservation struct {
	RunID                 string `json:"run_id"`
	Repetition            int    `json:"repetition"`
	Codec                 string `json:"codec"`
	Executor              string `json:"executor"`
	Limit                 int    `json:"k"`
	Workers               int    `json:"workers"`
	QueryOrdinal          int    `json:"query_ordinal"`
	ElapsedNS             int64  `json:"elapsed_ns"`
	Status                string `json:"status"`
	ResultCount           int    `json:"result_count"`
	ResultDigest          string `json:"result_digest"`
	QueryTerms            uint64 `json:"query_terms"`
	MatchedTerms          uint64 `json:"matched_terms"`
	CandidatesScored      uint64 `json:"candidates_scored"`
	PostingsDecoded       uint64 `json:"postings_decoded"`
	BlockHeadersRead      uint64 `json:"block_headers_read"`
	BlocksSkipped         uint64 `json:"blocks_skipped"`
	BlocksDecoded         uint64 `json:"blocks_decoded"`
	NextCalls             uint64 `json:"next_calls"`
	AdvanceCalls          uint64 `json:"advance_calls"`
	LogicalBytesRequested uint64 `json:"logical_bytes_requested"`
	PivotSelections       uint64 `json:"pivot_selections"`
	ThresholdChanges      uint64 `json:"threshold_changes"`
}

func runQueries(
	ctx context.Context,
	idx *indexfile.Index,
	queries []string,
	options queryOptions,
	output io.Writer,
) error {
	for ordinal := 0; ordinal < min(options.warmup, len(queries)); ordinal++ {
		if _, _, err := search.SearchWithStats(ctx, idx, queries[ordinal], options.limit, options.executor); err != nil {
			return fmt.Errorf("warm up query %d: %w", ordinal+1, err)
		}
	}

	codec, err := codecName(idx.PostingsCodec())
	if err != nil {
		return err
	}
	writer := json.NewEncoder(output)

	var queryErr error
	failedQueries := 0
	for ordinal, query := range queries {
		started := time.Now()
		results, stats, err := search.SearchWithStats(ctx, idx, query, options.limit, options.executor)
		elapsed := time.Since(started)

		observation := queryObservation{
			RunID:                 options.runID,
			Repetition:            options.repetition,
			Codec:                 codec,
			Executor:              options.executorName,
			Limit:                 options.limit,
			Workers:               1,
			QueryOrdinal:          ordinal + 1,
			ElapsedNS:             elapsed.Nanoseconds(),
			Status:                queryStatus(err),
			ResultCount:           len(results),
			QueryTerms:            stats.QueryTerms,
			MatchedTerms:          stats.MatchedTerms,
			CandidatesScored:      stats.CandidatesScored,
			PostingsDecoded:       stats.PostingsDecoded,
			BlockHeadersRead:      stats.BlockHeadersRead,
			BlocksSkipped:         stats.BlocksSkipped,
			BlocksDecoded:         stats.BlocksDecoded,
			NextCalls:             stats.NextCalls,
			AdvanceCalls:          stats.AdvanceCalls,
			LogicalBytesRequested: stats.LogicalBytesRequested,
			PivotSelections:       stats.PivotSelections,
			ThresholdChanges:      stats.ThresholdChanges,
		}
		if err == nil {
			observation.ResultDigest = digestResults(results)
		} else {
			failedQueries++
		}
		if err := writer.Encode(observation); err != nil {
			return fmt.Errorf("write query %d: %w", ordinal+1, err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			queryErr = err
			break
		}
	}

	if queryErr == nil && failedQueries != 0 {
		queryErr = fmt.Errorf("%d measured queries failed", failedQueries)
	}
	return queryErr
}

func digestResults(results []search.MeasuredResult) string {
	digest := sha256.New()
	digest.Write([]byte("diskseek-results-v1"))

	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], uint64(len(results)))
	digest.Write(encoded[:])
	for _, result := range results {
		binary.LittleEndian.PutUint32(encoded[:4], uint32(result.DocumentID))
		digest.Write(encoded[:4])
		binary.LittleEndian.PutUint64(encoded[:], uint64(len(result.ExternalID)))
		digest.Write(encoded[:])
		digest.Write([]byte(result.ExternalID))
		binary.LittleEndian.PutUint64(encoded[:], math.Float64bits(result.Score))
		digest.Write(encoded[:])
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func codecName(codec indexfile.PostingsCodec) (string, error) {
	switch codec {
	case indexfile.PostingsCodecRaw:
		return "raw", nil
	case indexfile.PostingsCodecVByte:
		return "vbyte", nil
	default:
		return "", fmt.Errorf("unsupported postings codec %d", codec)
	}
}

func queryStatus(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "search_error"
	}
}
