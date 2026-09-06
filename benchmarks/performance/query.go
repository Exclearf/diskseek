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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
)

type queryConfig struct {
	QueriesPath    string `json:"queries"`
	RawIndexPath   string `json:"raw_index"`
	VByteIndexPath string `json:"vbyte_index"`
	Repetitions    int    `json:"repetitions"`
	WarmupQueries  int    `json:"warmup_queries"`
}

type queryJob struct {
	QueriesPath string `json:"queries"`
	IndexPath   string `json:"index"`
	Repetition  int    `json:"repetition"`
	Executor    string `json:"executor"`
	Limit       int    `json:"k"`
	Warmup      int    `json:"warmup_queries"`
	RunsPath    string `json:"runs_path"`
}

type queryRunObservation struct {
	Repetition      int                `json:"repetition"`
	Codec           string             `json:"codec"`
	Executor        string             `json:"executor"`
	Limit           int                `json:"k"`
	QueryCount      int                `json:"query_count"`
	EngineElapsedNS int64              `json:"engine_elapsed_ns"`
	Process         queryProcessTotals `json:"process"`
}

type queryProcessTotals struct {
	UserCPUNS      int64  `json:"user_cpu_ns"`
	SystemCPUNS    int64  `json:"system_cpu_ns"`
	PeakRSSBytes   uint64 `json:"peak_rss_bytes"`
	AllocatedBytes uint64 `json:"allocated_bytes"`
	Allocations    uint64 `json:"allocations"`
	GCCycles       uint32 `json:"gc_cycles"`
	GCPauseNS      uint64 `json:"gc_pause_ns"`
}

func runQueryPlan(ctx context.Context, outputDirectory string, config queryConfig) error {
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return fmt.Errorf("create benchmark output directory: %w", err)
	}
	output, err := os.OpenFile(
		filepath.Join(outputDirectory, "queries.jsonl"),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create query observations: %w", err)
	}
	runsPath := filepath.Join(outputDirectory, "query_runs.jsonl")
	runs, err := os.OpenFile(runsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return errors.Join(fmt.Errorf("create query run observations: %w", err), output.Close())
	}
	if err := runs.Close(); err != nil {
		return errors.Join(err, output.Close())
	}

	executable, err := os.Executable()
	if err != nil {
		return errors.Join(fmt.Errorf("find benchmark executable: %w", err), output.Close())
	}
	scenarios := []queryJob{
		{IndexPath: config.RawIndexPath, Executor: "daat", Limit: 10},
		{IndexPath: config.RawIndexPath, Executor: "daat", Limit: 1000},
		{IndexPath: config.RawIndexPath, Executor: "wand", Limit: 10},
		{IndexPath: config.RawIndexPath, Executor: "wand", Limit: 1000},
		{IndexPath: config.VByteIndexPath, Executor: "daat", Limit: 10},
		{IndexPath: config.VByteIndexPath, Executor: "daat", Limit: 1000},
		{IndexPath: config.VByteIndexPath, Executor: "wand", Limit: 10},
		{IndexPath: config.VByteIndexPath, Executor: "wand", Limit: 1000},
	}
	for repetition := 1; repetition <= config.Repetitions; repetition++ {
		for position := range scenarios {
			job := scenarios[(position+repetition-1)%len(scenarios)]
			job.QueriesPath = config.QueriesPath
			job.Repetition = repetition
			job.Warmup = config.WarmupQueries
			job.RunsPath = runsPath
			if err := runQueryProcess(ctx, executable, job, output); err != nil {
				return errors.Join(err, output.Close())
			}
		}
	}
	if err := output.Close(); err != nil {
		return err
	}
	return writeQueryResults(outputDirectory)
}

func runQueryProcess(ctx context.Context, executable string, job queryJob, output io.Writer) error {
	if err := primeIndex(job.IndexPath); err != nil {
		return fmt.Errorf("prime query index: %w", err)
	}
	encodedJob, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode query job: %w", err)
	}

	command := exec.CommandContext(ctx, executable)
	command.Env = append(os.Environ(), queryJobEnvironment+"="+string(encodedJob))
	command.Stdout = output
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("query %s k=%d repetition %d: %w", job.Executor, job.Limit, job.Repetition, err)
	}
	return nil
}

func runQueryJob(ctx context.Context, encodedJob string, output io.Writer) error {
	var job queryJob
	if err := json.Unmarshal([]byte(encodedJob), &job); err != nil {
		return fmt.Errorf("decode query job: %w", err)
	}
	executor, err := parseQueryExecutor(job.Executor)
	if err != nil {
		return err
	}
	queries, err := readQueries(job.QueriesPath)
	if err != nil {
		return err
	}
	idx, err := indexfile.Open(job.IndexPath)
	if err != nil {
		return fmt.Errorf("open query index: %w", err)
	}
	defer idx.Close()
	run, err := runQueries(ctx, idx, queries, queryOptions{
		repetition:   job.Repetition,
		executor:     executor,
		executorName: job.Executor,
		limit:        job.Limit,
		warmup:       job.Warmup,
	}, output)
	if err != nil {
		return err
	}
	return writeQueryRun(job.RunsPath, run)
}

func writeQueryRun(path string, run queryRunObservation) error {
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Errorf("open query run observations: %w", err)
	}
	return errors.Join(json.NewEncoder(output).Encode(run), output.Close())
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

func primeIndex(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := primeFile(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func parseQueryExecutor(name string) (search.Executor, error) {
	switch name {
	case "daat":
		return search.ExecutorDAAT, nil
	case "wand":
		return search.ExecutorWAND, nil
	default:
		return 0, fmt.Errorf("unsupported query executor %q", name)
	}
}

type queryOptions struct {
	repetition   int
	executor     search.Executor
	executorName string
	limit        int
	warmup       int
}

type queryObservation struct {
	Repetition            int    `json:"repetition"`
	Codec                 string `json:"codec"`
	Executor              string `json:"executor"`
	Limit                 int    `json:"k"`
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
) (queryRunObservation, error) {
	for ordinal := 0; ordinal < min(options.warmup, len(queries)); ordinal++ {
		if _, _, err := search.SearchWithStats(ctx, idx, queries[ordinal], options.limit, options.executor); err != nil {
			return queryRunObservation{}, fmt.Errorf("warm up query %d: %w", ordinal+1, err)
		}
	}

	codec, err := codecName(idx.PostingsCodec())
	if err != nil {
		return queryRunObservation{}, err
	}
	writer := json.NewEncoder(output)
	var beforeMemory runtime.MemStats
	runtime.ReadMemStats(&beforeMemory)
	beforeUsage, err := readProcessUsage()
	if err != nil {
		return queryRunObservation{}, err
	}

	var engineElapsed time.Duration
	for ordinal, query := range queries {
		started := time.Now()
		results, stats, err := search.SearchWithStats(ctx, idx, query, options.limit, options.executor)
		elapsed := time.Since(started)
		engineElapsed += elapsed

		observation := queryObservation{
			Repetition:            options.repetition,
			Codec:                 codec,
			Executor:              options.executorName,
			Limit:                 options.limit,
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
		}
		if err := writer.Encode(observation); err != nil {
			return queryRunObservation{}, fmt.Errorf("write query %d: %w", ordinal+1, err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return queryRunObservation{}, err
		}
	}

	afterUsage, err := readProcessUsage()
	if err != nil {
		return queryRunObservation{}, err
	}
	var afterMemory runtime.MemStats
	runtime.ReadMemStats(&afterMemory)
	return queryRunObservation{
		Repetition:      options.repetition,
		Codec:           codec,
		Executor:        options.executorName,
		Limit:           options.limit,
		QueryCount:      len(queries),
		EngineElapsedNS: engineElapsed.Nanoseconds(),
		Process: queryProcessTotals{
			UserCPUNS:      (afterUsage.userCPU - beforeUsage.userCPU).Nanoseconds(),
			SystemCPUNS:    (afterUsage.systemCPU - beforeUsage.systemCPU).Nanoseconds(),
			PeakRSSBytes:   afterUsage.peakRSS,
			AllocatedBytes: afterMemory.TotalAlloc - beforeMemory.TotalAlloc,
			Allocations:    afterMemory.Mallocs - beforeMemory.Mallocs,
			GCCycles:       afterMemory.NumGC - beforeMemory.NumGC,
			GCPauseNS:      afterMemory.PauseTotalNs - beforeMemory.PauseTotalNs,
		},
	}, nil
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
