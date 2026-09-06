package main

import (
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Exclearf/diskseek/demo/internal/simplewiki"
	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/segment"
	"github.com/spf13/cobra"
)

const catalogFileName = "catalog.jsonl"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := newCommand()
	command.SetContext(ctx)
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prepare-simplewiki DUMP DESTINATION",
		Short: "Build a demo dataset from a Simple English Wikipedia dump",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			stats, err := prepare(command.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"built %d documents; skipped %d namespaces, %d empty, %d oversized\n",
				stats.Documents,
				stats.SkippedNamespaces,
				stats.SkippedEmpty,
				stats.SkippedOversized,
			)
			return err
		},
	}
}

func prepare(ctx context.Context, sourcePath, destination string) (simplewiki.Stats, error) {
	if _, err := os.Lstat(destination); err == nil {
		return simplewiki.Stats{}, errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return simplewiki.Stats{}, fmt.Errorf("inspect destination: %w", err)
	}

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return simplewiki.Stats{}, fmt.Errorf("create destination parent: %w", err)
	}
	workDirectory, err := os.MkdirTemp(parent, ".diskseek-simplewiki-*")
	if err != nil {
		return simplewiki.Stats{}, fmt.Errorf("create working directory: %w", err)
	}
	defer os.RemoveAll(workDirectory)

	corpusPath := filepath.Join(workDirectory, "corpus.tsv")
	catalogPath := filepath.Join(workDirectory, catalogFileName)
	stats, err := convertDump(ctx, sourcePath, corpusPath, catalogPath)
	if err != nil {
		return simplewiki.Stats{}, fmt.Errorf("convert dump: %w", err)
	}

	corpusInput, err := os.Open(corpusPath)
	if err != nil {
		return simplewiki.Stats{}, fmt.Errorf("open generated corpus: %w", err)
	}
	bundlePath := filepath.Join(workDirectory, "bundle")
	_, buildErr := segment.BuildIndex(
		ctx,
		corpus.NewTSVReader(corpusInput),
		bundlePath,
		segment.BuildOptions{
			FlushTarget:        64 << 20,
			MergeFanIn:         16,
			MergeWorkers:       1,
			Codec:              indexfile.PostingsCodecVByte,
			TemporaryDirectory: workDirectory,
		},
	)
	if err := errors.Join(buildErr, corpusInput.Close()); err != nil {
		return simplewiki.Stats{}, fmt.Errorf("build index: %w", err)
	}

	if err := os.Rename(catalogPath, filepath.Join(bundlePath, catalogFileName)); err != nil {
		return simplewiki.Stats{}, fmt.Errorf("install catalog: %w", err)
	}
	if err := os.Rename(bundlePath, destination); err != nil {
		return simplewiki.Stats{}, fmt.Errorf("publish dataset: %w", err)
	}
	return stats, nil
}

func convertDump(ctx context.Context, sourcePath, corpusPath, catalogPath string) (stats simplewiki.Stats, err error) {
	input, err := os.Open(sourcePath)
	if err != nil {
		return simplewiki.Stats{}, err
	}
	defer func() { err = errors.Join(err, input.Close()) }()

	corpusOutput, err := os.Create(corpusPath)
	if err != nil {
		return simplewiki.Stats{}, err
	}
	defer func() { err = errors.Join(err, corpusOutput.Close()) }()

	catalogOutput, err := os.Create(catalogPath)
	if err != nil {
		return simplewiki.Stats{}, err
	}
	defer func() { err = errors.Join(err, catalogOutput.Close()) }()

	return simplewiki.Convert(ctx, bzip2.NewReader(input), corpusOutput, catalogOutput)
}
