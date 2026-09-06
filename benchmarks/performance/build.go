package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/segment"
)

const formatPostingsPerBlock = 128

type buildOptions struct {
	runID      string
	repetition int
	codecName  string
	build      segment.BuildOptions
}

type buildObservation struct {
	RunID                    string `json:"run_id"`
	Repetition               int    `json:"repetition"`
	Codec                    string `json:"codec"`
	FlushTargetBytes         uint64 `json:"flush_target_bytes"`
	MergeFanIn               int    `json:"merge_fan_in"`
	MergeWorkers             int    `json:"merge_workers"`
	PostingsPerBlock         int    `json:"postings_per_block"`
	ElapsedNS                int64  `json:"elapsed_ns"`
	UserCPUNS                int64  `json:"user_cpu_ns"`
	SystemCPUNS              int64  `json:"system_cpu_ns"`
	Status                   string `json:"status"`
	Documents                uint64 `json:"documents"`
	DocumentsWithTerms       uint64 `json:"documents_with_terms"`
	Tokens                   uint64 `json:"tokens"`
	MaxAccountedSegmentBytes uint64 `json:"max_accounted_segment_bytes"`
	PeakRSSBytes             uint64 `json:"peak_rss_bytes"`
	FinalIndexBytes          uint64 `json:"final_index_bytes"`
	TermsBytes               uint64 `json:"terms_bytes"`
	PostingsBytes            uint64 `json:"postings_bytes"`
	DocumentLengthsBytes     uint64 `json:"document_lengths_bytes"`
	DocumentOffsetsBytes     uint64 `json:"document_offsets_bytes"`
	ExternalIDsBytes         uint64 `json:"external_ids_bytes"`
	PostingsCount            uint64 `json:"postings_count"`
	RunCount                 uint64 `json:"run_count"`
	MergePasses              uint64 `json:"merge_passes"`
	MergeInputBytes          uint64 `json:"merge_input_bytes"`
	MergeOutputBytes         uint64 `json:"merge_output_bytes"`
	IndexDigest              string `json:"index_digest"`
}

type processUsage struct {
	userCPU   time.Duration
	systemCPU time.Duration
	peakRSS   uint64
}

type indexArtifact struct {
	metadataBytes        uint64
	termsBytes           uint64
	postingsBytes        uint64
	documentLengthsBytes uint64
	documentOffsetsBytes uint64
	externalIDsBytes     uint64
	totalBytes           uint64
	digest               string
}

func runBuildJob(ctx context.Context, encodedJob string, output io.Writer) error {
	var job buildJob
	if err := json.Unmarshal([]byte(encodedJob), &job); err != nil {
		return fmt.Errorf("decode build job: %w", err)
	}
	if err := primeFile(job.CorpusPath); err != nil {
		return fmt.Errorf("prime corpus: %w", err)
	}

	codec, err := parseBuildCodec(job.Codec)
	if err != nil {
		return err
	}
	return runBuild(ctx, job.CorpusPath, job.Destination, buildOptions{
		runID:      job.RunID,
		repetition: job.Repetition,
		codecName:  job.Codec,
		build: segment.BuildOptions{
			FlushTarget:        job.FlushTarget,
			MergeFanIn:         job.MergeFanIn,
			MergeWorkers:       job.MergeWorkers,
			Codec:              codec,
			TemporaryDirectory: job.TemporaryDirectory,
		},
	}, output)
}

func primeFile(path string) error {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(io.Discard, input)
	return errors.Join(copyErr, input.Close())
}

func runBuild(
	ctx context.Context,
	corpusPath string,
	destination string,
	options buildOptions,
	output io.Writer,
) error {
	input, err := os.Open(corpusPath)
	if err != nil {
		return fmt.Errorf("open corpus: %w", err)
	}
	defer input.Close()

	before, err := readProcessUsage()
	if err != nil {
		return err
	}
	started := time.Now()
	report, buildErr := segment.BuildIndex(
		ctx,
		corpus.NewTSVReader(input),
		destination,
		options.build,
	)
	elapsed := time.Since(started)
	after, err := readProcessUsage()
	if err != nil {
		return errors.Join(buildErr, err)
	}

	observation := buildObservation{
		RunID:                    options.runID,
		Repetition:               options.repetition,
		Codec:                    options.codecName,
		FlushTargetBytes:         options.build.FlushTarget,
		MergeFanIn:               options.build.MergeFanIn,
		MergeWorkers:             options.build.MergeWorkers,
		PostingsPerBlock:         formatPostingsPerBlock,
		ElapsedNS:                elapsed.Nanoseconds(),
		UserCPUNS:                (after.userCPU - before.userCPU).Nanoseconds(),
		SystemCPUNS:              (after.systemCPU - before.systemCPU).Nanoseconds(),
		Status:                   "build_error",
		Documents:                report.Documents,
		DocumentsWithTerms:       report.DocumentsWithTerms,
		Tokens:                   report.Tokens,
		MaxAccountedSegmentBytes: report.MaxAccountedSegmentBytes,
		PeakRSSBytes:             after.peakRSS,
		PostingsCount:            report.Postings,
		RunCount:                 report.RunCount,
		MergePasses:              report.MergePasses,
		MergeInputBytes:          report.MergeInputBytes,
		MergeOutputBytes:         report.MergeOutputBytes,
	}
	runErr := buildErr
	if buildErr == nil {
		observation.Status = "artifact_error"
		artifact, inspectErr := inspectIndex(destination)
		runErr = inspectErr
		observation.FinalIndexBytes = artifact.totalBytes
		observation.TermsBytes = artifact.termsBytes
		observation.PostingsBytes = artifact.postingsBytes
		observation.DocumentLengthsBytes = artifact.documentLengthsBytes
		observation.DocumentOffsetsBytes = artifact.documentOffsetsBytes
		observation.ExternalIDsBytes = artifact.externalIDsBytes
		observation.IndexDigest = artifact.digest
	}
	if runErr == nil {
		observation.Status = "verify_error"
		runErr = indexfile.Verify(ctx, destination)
	}
	if runErr == nil {
		observation.Status = "ok"
	}

	if err := json.NewEncoder(output).Encode(observation); err != nil {
		return errors.Join(runErr, fmt.Errorf("write build row: %w", err))
	}
	return runErr
}

func readProcessUsage() (processUsage, error) {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return processUsage{}, fmt.Errorf("read process usage: %w", err)
	}
	return processUsage{
		userCPU:   time.Duration(syscall.TimevalToNsec(usage.Utime)),
		systemCPU: time.Duration(syscall.TimevalToNsec(usage.Stime)),
		peakRSS:   uint64(usage.Maxrss) * 1024,
	}, nil
}

func inspectIndex(directory string) (indexArtifact, error) {
	var artifact indexArtifact
	files := [...]struct {
		name string
		size *uint64
	}{
		{indexfile.MetadataFileName, &artifact.metadataBytes},
		{indexfile.TermsFileName, &artifact.termsBytes},
		{indexfile.PostingsFileName, &artifact.postingsBytes},
		{indexfile.DocumentLengthsFileName, &artifact.documentLengthsBytes},
		{indexfile.DocumentOffsetsFileName, &artifact.documentOffsetsBytes},
		{indexfile.DocumentDataFileName, &artifact.externalIDsBytes},
	}
	combined := sha256.New()
	for _, file := range files {
		input, err := os.Open(filepath.Join(directory, file.name))
		if err != nil {
			return indexArtifact{}, fmt.Errorf("open %s: %w", file.name, err)
		}
		digest := sha256.New()
		written, copyErr := io.Copy(digest, input)
		closeErr := input.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return indexArtifact{}, fmt.Errorf("hash %s: %w", file.name, err)
		}
		*file.size = uint64(written)
		artifact.totalBytes += uint64(written)
		combined.Write(digest.Sum(nil))
	}
	artifact.digest = hex.EncodeToString(combined.Sum(nil))
	return artifact, nil
}

func parseBuildCodec(name string) (indexfile.PostingsCodec, error) {
	switch name {
	case "raw":
		return indexfile.PostingsCodecRaw, nil
	case "vbyte":
		return indexfile.PostingsCodecVByte, nil
	default:
		return 0, fmt.Errorf("unsupported postings codec %q", name)
	}
}
