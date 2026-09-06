package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const bytesPerMiB = 1024 * 1024

type benchmarkResults struct {
	Builds []buildResult `json:"builds"`
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

func writeResults(directory string) error {
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
