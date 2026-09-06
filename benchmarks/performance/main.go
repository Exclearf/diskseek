package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/spf13/cobra"
)

var queryHeader = []string{
	"run_id",
	"repetition",
	"codec",
	"executor",
	"k",
	"workers",
	"query_ordinal",
	"elapsed_ns",
	"status",
	"result_count",
	"result_digest",
	"query_terms",
	"matched_terms",
	"candidates_scored",
	"postings_decoded",
	"block_headers_read",
	"blocks_skipped",
	"blocks_decoded",
	"next_calls",
	"advance_calls",
	"logical_bytes_requested",
	"pivot_selections",
	"threshold_changes",
}

type queryOptions struct {
	runID        string
	repetition   int
	executor     search.Executor
	executorName string
	limit        int
	warmup       int
}

type queryObservation struct {
	runID        string
	repetition   int
	codec        string
	executor     string
	limit        int
	ordinal      int
	elapsed      time.Duration
	status       string
	resultCount  int
	resultDigest string
	stats        search.QueryStats
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	command := newCommand()
	command.SetContext(ctx)
	status := 0
	if err := command.Execute(); err != nil {
		status = 1
	}
	stop()
	os.Exit(status)
}

func newCommand() *cobra.Command {
	var options queryOptions
	command := &cobra.Command{
		Use:   "diskseek-query-bench INDEX QUERIES",
		Short: "Measure DiskSeek query execution",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, paths []string) error {
			if options.runID == "" {
				return errors.New("run ID is required")
			}
			if options.repetition < 1 {
				return errors.New("repetition must be positive")
			}
			if options.limit < 1 {
				return errors.New("query limit must be positive")
			}
			if options.warmup < 0 {
				return errors.New("warm-up query count cannot be negative")
			}

			executor, err := parseExecutor(options.executorName)
			if err != nil {
				return err
			}
			options.executor = executor

			queries, err := readQueries(paths[1])
			if err != nil {
				return err
			}
			idx, err := indexfile.Open(paths[0])
			if err != nil {
				return fmt.Errorf("open index: %w", err)
			}
			return errors.Join(
				runQueries(command.Context(), idx, queries, options, command.OutOrStdout()),
				idx.Close(),
			)
		},
	}
	command.Flags().StringVar(&options.runID, "run-id", "", "benchmark run identifier")
	command.Flags().IntVar(&options.repetition, "repetition", 0, "process repetition number")
	command.Flags().StringVar(&options.executorName, "executor", "", "query executor: daat or wand")
	command.Flags().IntVar(&options.limit, "limit", 10, "maximum results per query")
	command.Flags().IntVar(&options.warmup, "warmup", 500, "untimed warm-up queries")
	return command
}

func readQueries(path string) ([]string, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open queries: %w", err)
	}
	defer input.Close()

	reader := corpus.NewTSVReader(input)
	var queries []string
	for {
		record, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return queries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read queries: %w", err)
		}
		queries = append(queries, record.Text)
	}
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
	writer := csv.NewWriter(output)
	writer.Comma = '\t'
	if err := writer.Write(queryHeader); err != nil {
		return fmt.Errorf("write query header: %w", err)
	}

	var queryErr error
	failedQueries := 0
	for ordinal, query := range queries {
		started := time.Now()
		results, stats, err := search.SearchWithStats(ctx, idx, query, options.limit, options.executor)
		elapsed := time.Since(started)

		observation := queryObservation{
			runID:       options.runID,
			repetition:  options.repetition,
			codec:       codec,
			executor:    options.executorName,
			limit:       options.limit,
			ordinal:     ordinal + 1,
			elapsed:     elapsed,
			status:      queryStatus(err),
			resultCount: len(results),
			stats:       stats,
		}
		if err == nil {
			observation.resultDigest = digestResults(results)
		} else {
			failedQueries++
		}
		if err := writer.Write(observation.record()); err != nil {
			return fmt.Errorf("write query %d: %w", ordinal+1, err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			queryErr = err
			break
		}
	}

	writer.Flush()
	if queryErr == nil && failedQueries != 0 {
		queryErr = fmt.Errorf("%d measured queries failed", failedQueries)
	}
	return errors.Join(queryErr, writer.Error())
}

func (o queryObservation) record() []string {
	return []string{
		o.runID,
		strconv.Itoa(o.repetition),
		o.codec,
		o.executor,
		strconv.Itoa(o.limit),
		"1",
		strconv.Itoa(o.ordinal),
		strconv.FormatInt(o.elapsed.Nanoseconds(), 10),
		o.status,
		strconv.Itoa(o.resultCount),
		o.resultDigest,
		strconv.FormatUint(o.stats.QueryTerms, 10),
		strconv.FormatUint(o.stats.MatchedTerms, 10),
		strconv.FormatUint(o.stats.CandidatesScored, 10),
		strconv.FormatUint(o.stats.PostingsDecoded, 10),
		strconv.FormatUint(o.stats.BlockHeadersRead, 10),
		strconv.FormatUint(o.stats.BlocksSkipped, 10),
		strconv.FormatUint(o.stats.BlocksDecoded, 10),
		strconv.FormatUint(o.stats.NextCalls, 10),
		strconv.FormatUint(o.stats.AdvanceCalls, 10),
		strconv.FormatUint(o.stats.LogicalBytesRequested, 10),
		strconv.FormatUint(o.stats.PivotSelections, 10),
		strconv.FormatUint(o.stats.ThresholdChanges, 10),
	}
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

func parseExecutor(name string) (search.Executor, error) {
	switch name {
	case "wand":
		return search.ExecutorWAND, nil
	case "daat":
		return search.ExecutorDAAT, nil
	default:
		return 0, fmt.Errorf("unsupported query executor %q", name)
	}
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
