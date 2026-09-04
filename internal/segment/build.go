package segment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Exclearf/diskseek/internal/corpus"
)

type buildResult struct {
	directory     string
	documentsPath string
	runPaths      []string
	stats         buildStats
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
