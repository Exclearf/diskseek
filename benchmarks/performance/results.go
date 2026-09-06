package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
)

const bytesPerMiB = 1024 * 1024

type benchmarkResults struct {
	Builds  []buildResult `json:"builds,omitempty"`
	Queries []queryResult `json:"queries,omitempty"`
}

type buildResult struct {
	Codec                 string  `json:"codec"`
	Repetition            int     `json:"repetition"`
	ElapsedSeconds        float64 `json:"elapsed_seconds"`
	DocumentsPerSecond    float64 `json:"documents_per_second"`
	TokensPerSecond       float64 `json:"tokens_per_second"`
	PostingsPerSecond     float64 `json:"postings_per_second"`
	InputMiBPerSecond     float64 `json:"input_mib_per_second"`
	IndexMiB              float64 `json:"index_mib"`
	IndexBytesPerPosting  float64 `json:"index_bytes_per_posting"`
	PeakResidentMemoryMiB float64 `json:"peak_resident_memory_mib"`
}

type queryResult struct {
	Repetition       int                `json:"repetition"`
	Codec            string             `json:"codec"`
	Executor         string             `json:"executor"`
	Limit            int                `json:"k"`
	QueryCount       int                `json:"query_count"`
	Failures         int                `json:"failures"`
	ShortResults     int                `json:"short_results"`
	EngineElapsedNS  int64              `json:"engine_elapsed_ns"`
	QueriesPerSecond float64            `json:"queries_per_second"`
	Latency          queryLatency       `json:"latency_ns"`
	Work             queryWork          `json:"work"`
	Process          queryProcessTotals `json:"process"`
}

type queryLatency struct {
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	P99 int64 `json:"p99"`
	Max int64 `json:"max"`
}

type queryWork struct {
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

type queryAccumulator struct {
	result  queryResult
	elapsed []int64
}

func writeBuildResults(directory string) error {
	input, err := os.Open(filepath.Join(directory, "builds.jsonl"))
	if err != nil {
		return fmt.Errorf("open build observations: %w", err)
	}

	var results benchmarkResults
	decoder := json.NewDecoder(input)
	for {
		var observation buildObservation
		if err := decoder.Decode(&observation); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return errors.Join(fmt.Errorf("read build observation: %w", err), input.Close())
		}
		results.Builds = append(results.Builds, summarizeBuild(observation))
	}
	if err := input.Close(); err != nil {
		return err
	}

	return writeResults(directory, results)
}

func writeQueryResults(directory string) error {
	input, err := os.Open(filepath.Join(directory, "queries.jsonl"))
	if err != nil {
		return fmt.Errorf("open query observations: %w", err)
	}

	var results benchmarkResults
	var current queryAccumulator
	decoder := json.NewDecoder(input)
	for {
		var observation queryObservation
		if err := decoder.Decode(&observation); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return errors.Join(fmt.Errorf("read query observation: %w", err), input.Close())
		}

		if current.result.QueryCount != 0 && !current.matches(observation) {
			results.Queries = append(results.Queries, current.summarize())
			current = queryAccumulator{}
		}
		current.add(observation)
	}
	if current.result.QueryCount != 0 {
		results.Queries = append(results.Queries, current.summarize())
	}
	if err := input.Close(); err != nil {
		return err
	}
	runs, err := readQueryRuns(directory)
	if err != nil {
		return err
	}
	if len(runs) != len(results.Queries) {
		return errors.New("query run count does not match query summaries")
	}
	for resultIndex, run := range runs {
		result := &results.Queries[resultIndex]
		if !result.matches(run) {
			return errors.New("query run identity does not match query summary")
		}
		result.EngineElapsedNS = run.EngineElapsedNS
		result.QueriesPerSecond = float64(result.QueryCount) / (float64(run.EngineElapsedNS) / 1e9)
		result.Process = run.Process
	}
	return writeResults(directory, results)
}

func readQueryRuns(directory string) ([]queryRunObservation, error) {
	input, err := os.Open(filepath.Join(directory, "query_runs.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("open query run observations: %w", err)
	}

	var runs []queryRunObservation
	decoder := json.NewDecoder(input)
	for {
		var run queryRunObservation
		if err := decoder.Decode(&run); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, errors.Join(fmt.Errorf("read query run observation: %w", err), input.Close())
		}
		runs = append(runs, run)
	}
	return runs, input.Close()
}

func writeResults(directory string, results benchmarkResults) error {
	output, err := os.OpenFile(
		filepath.Join(directory, "results.json"),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o644,
	)
	if err != nil {
		return fmt.Errorf("create benchmark results: %w", err)
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return errors.Join(encoder.Encode(results), output.Close())
}

func (a *queryAccumulator) matches(observation queryObservation) bool {
	return a.result.Repetition == observation.Repetition &&
		a.result.Codec == observation.Codec &&
		a.result.Executor == observation.Executor &&
		a.result.Limit == observation.Limit
}

func (r queryResult) matches(run queryRunObservation) bool {
	return r.Repetition == run.Repetition &&
		r.Codec == run.Codec &&
		r.Executor == run.Executor &&
		r.Limit == run.Limit &&
		r.QueryCount == run.QueryCount
}

func (a *queryAccumulator) add(observation queryObservation) {
	if a.result.QueryCount == 0 {
		a.result.Repetition = observation.Repetition
		a.result.Codec = observation.Codec
		a.result.Executor = observation.Executor
		a.result.Limit = observation.Limit
	}

	a.result.QueryCount++
	if observation.Status != "ok" {
		a.result.Failures++
	} else if observation.ResultCount < observation.Limit {
		a.result.ShortResults++
	}
	a.elapsed = append(a.elapsed, observation.ElapsedNS)
	a.result.Work.QueryTerms += observation.QueryTerms
	a.result.Work.MatchedTerms += observation.MatchedTerms
	a.result.Work.CandidatesScored += observation.CandidatesScored
	a.result.Work.PostingsDecoded += observation.PostingsDecoded
	a.result.Work.BlockHeadersRead += observation.BlockHeadersRead
	a.result.Work.BlocksSkipped += observation.BlocksSkipped
	a.result.Work.BlocksDecoded += observation.BlocksDecoded
	a.result.Work.NextCalls += observation.NextCalls
	a.result.Work.AdvanceCalls += observation.AdvanceCalls
	a.result.Work.LogicalBytesRequested += observation.LogicalBytesRequested
	a.result.Work.PivotSelections += observation.PivotSelections
	a.result.Work.ThresholdChanges += observation.ThresholdChanges
}

func (a *queryAccumulator) summarize() queryResult {
	slices.Sort(a.elapsed)
	a.result.Latency = queryLatency{
		P50: nearestRank(a.elapsed, 50),
		P95: nearestRank(a.elapsed, 95),
		P99: nearestRank(a.elapsed, 99),
		Max: a.elapsed[len(a.elapsed)-1],
	}
	return a.result
}

func nearestRank(sorted []int64, percentile int) int64 {
	index := (len(sorted)*percentile+99)/100 - 1
	return sorted[index]
}

func summarizeBuild(observation buildObservation) buildResult {
	elapsedSeconds := float64(observation.ElapsedNS) / 1e9
	return buildResult{
		Codec:                 observation.Codec,
		Repetition:            observation.Repetition,
		ElapsedSeconds:        elapsedSeconds,
		DocumentsPerSecond:    float64(observation.Documents) / elapsedSeconds,
		TokensPerSecond:       float64(observation.Tokens) / elapsedSeconds,
		PostingsPerSecond:     float64(observation.PostingsCount) / elapsedSeconds,
		InputMiBPerSecond:     float64(observation.CorpusBytes) / bytesPerMiB / elapsedSeconds,
		IndexMiB:              float64(observation.FinalIndexBytes) / bytesPerMiB,
		IndexBytesPerPosting:  float64(observation.FinalIndexBytes) / float64(observation.PostingsCount),
		PeakResidentMemoryMiB: float64(observation.PeakRSSBytes) / bytesPerMiB,
	}
}
