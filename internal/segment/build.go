package segment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

type BuildOptions struct {
	FlushTarget        uint64
	MergeFanIn         int
	MergeWorkers       int
	Codec              indexfile.PostingsCodec
	TemporaryDirectory string
}

type BuildReport struct {
	Documents                uint64
	DocumentsWithTerms       uint64
	Tokens                   uint64
	Postings                 uint64
	MaxAccountedSegmentBytes uint64
	RunCount                 uint64
	MergePasses              uint64
	MergeInputBytes          uint64
	MergeOutputBytes         uint64
}

type buildResult struct {
	directory     string
	documentsPath string
	runPaths      []string
	stats         buildStats
}

// BuildIndex builds a new persistent index directory from TSV records.
func BuildIndex(
	ctx context.Context,
	records *corpus.TSVReader,
	destination string,
	options BuildOptions,
) (report BuildReport, err error) {
	if err := validateBuildOptions(options); err != nil {
		return BuildReport{}, err
	}

	result, err := build(ctx, records, options.FlushTarget, options.TemporaryDirectory)
	if err != nil {
		return BuildReport{}, fmt.Errorf("build runs: %w", err)
	}
	defer func() {
		err = errors.Join(err, os.RemoveAll(result.directory))
	}()

	mergedRun, mergeStats, err := mergeRuns(
		ctx,
		result.directory,
		result.runPaths,
		options.MergeFanIn,
		options.MergeWorkers,
	)
	if err != nil {
		return BuildReport{}, fmt.Errorf("merge runs: %w", err)
	}
	if err := writeIndex(ctx, destination, mergedRun, result.documentsPath, options.Codec); err != nil {
		return BuildReport{}, fmt.Errorf("write index: %w", err)
	}

	report = BuildReport{
		Documents:                result.stats.documentCount,
		DocumentsWithTerms:       result.stats.documentsWithTerms,
		Tokens:                   result.stats.totalTokenCount,
		Postings:                 result.stats.postingCount,
		MaxAccountedSegmentBytes: result.stats.maxAccountedBytes,
		RunCount:                 uint64(len(result.runPaths)),
	}
	if len(mergeStats) != 0 {
		report.MergePasses = uint64(mergeStats[len(mergeStats)-1].passIndex + 1)
	}
	for _, stats := range mergeStats {
		if stats.inputCount == 1 {
			continue
		}
		report.MergeInputBytes += stats.inputBytes
		report.MergeOutputBytes += stats.outputBytes
	}
	return report, nil
}

func validateBuildOptions(options BuildOptions) error {
	if options.FlushTarget == 0 {
		return errors.New("segment flush target must be positive")
	}
	if options.MergeFanIn < 2 {
		return errors.New("merge fan-in must be at least two")
	}
	if options.MergeWorkers < 1 {
		return errors.New("merge worker count must be positive")
	}
	if options.Codec != indexfile.PostingsCodecRaw && options.Codec != indexfile.PostingsCodecVByte {
		return errors.New("unsupported postings codec")
	}
	return nil
}

func build(
	ctx context.Context,
	records *corpus.TSVReader,
	flushTarget uint64,
	temporaryDirectory string,
) (result buildResult, err error) {
	if flushTarget == 0 {
		return buildResult{}, errors.New("segment flush target must be positive")
	}
	if err := ctx.Err(); err != nil {
		return buildResult{}, err
	}

	directory, err := os.MkdirTemp(temporaryDirectory, "diskseek-build-*")
	if err != nil {
		return buildResult{}, fmt.Errorf("create build directory: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(directory))
		}
	}()

	documents, err := os.CreateTemp(directory, "documents-*")
	if err != nil {
		return buildResult{}, fmt.Errorf("create document sidecar: %w", err)
	}

	var runPaths []string
	stats, err := buildRuns(ctx, records, flushTarget, documents, func() (io.WriteCloser, error) {
		run, err := os.CreateTemp(directory, "run-*")
		if err != nil {
			return nil, fmt.Errorf("create run: %w", err)
		}
		runPaths = append(runPaths, run.Name())
		return run, nil
	})
	if err != nil {
		return buildResult{}, err
	}

	return buildResult{
		directory:     directory,
		documentsPath: documents.Name(),
		runPaths:      runPaths,
		stats:         stats,
	}, nil
}
