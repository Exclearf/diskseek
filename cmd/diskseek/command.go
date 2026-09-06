package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/Exclearf/diskseek/internal/segment"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

const (
	defaultFlushTarget  uint64 = 64 << 20
	defaultMergeFanIn          = 16
	defaultMergeWorkers        = 1
)

func newRootCommand(version string, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "diskseek",
		Short: "A disk-backed search engine",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(newIndexCommand())
	command.AddCommand(newQueryCommand())
	command.AddCommand(newVerifyCommand())
	command.AddCommand(newVersionCommand(version))
	return command
}

func newIndexCommand() *cobra.Command {
	codecName := "vbyte"
	options := segment.BuildOptions{
		FlushTarget:  defaultFlushTarget,
		MergeFanIn:   defaultMergeFanIn,
		MergeWorkers: defaultMergeWorkers,
	}
	command := &cobra.Command{
		Use:   "index CORPUS DESTINATION",
		Short: "Build an index from a TSV corpus",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			codec, err := parseCodec(codecName)
			if err != nil {
				return err
			}
			options.Codec = codec

			input, err := os.Open(args[0])
			if err != nil {
				return fmt.Errorf("open corpus: %w", err)
			}
			defer input.Close()

			if err := segment.BuildIndex(
				command.Context(),
				corpus.NewTSVReader(input),
				args[1],
				options,
			); err != nil {
				return fmt.Errorf("build index: %w", err)
			}
			return nil
		},
	}
	command.Flags().Uint64Var(&options.FlushTarget, "flush-target", options.FlushTarget, "segment flush target in bytes")
	command.Flags().IntVar(&options.MergeFanIn, "merge-fan-in", options.MergeFanIn, "maximum input runs per merge")
	command.Flags().IntVar(&options.MergeWorkers, "merge-workers", options.MergeWorkers, "concurrent merge workers")
	command.Flags().StringVar(&codecName, "codec", codecName, "postings codec: raw or vbyte")
	command.Flags().StringVar(&options.TemporaryDirectory, "temp-dir", "", "directory for temporary build files")
	return command
}

func parseCodec(name string) (indexfile.PostingsCodec, error) {
	switch name {
	case "raw":
		return indexfile.PostingsCodecRaw, nil
	case "vbyte":
		return indexfile.PostingsCodecVByte, nil
	default:
		return 0, fmt.Errorf("unsupported postings codec %q", name)
	}
}

func newQueryCommand() *cobra.Command {
	limit := 10
	command := &cobra.Command{
		Use:   "query INDEX QUERY",
		Short: "Search an index",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if limit <= 0 {
				return errors.New("query limit must be positive")
			}

			idx, err := indexfile.Open(args[0])
			if err != nil {
				return fmt.Errorf("open index: %w", err)
			}
			defer idx.Close()

			results, err := search.Search(command.Context(), idx, args[1], limit)
			if err != nil {
				return fmt.Errorf("search index: %w", err)
			}
			for _, result := range results {
				score := strconv.FormatFloat(result.Score, 'g', -1, 64)
				if _, err := fmt.Fprintf(command.OutOrStdout(), "%s\t%s\n", result.ExternalID, score); err != nil {
					return fmt.Errorf("write result: %w", err)
				}
			}
			return nil
		},
	}
	command.Flags().IntVar(&limit, "limit", limit, "maximum number of results")
	return command
}

func newVerifyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "verify INDEX",
		Short: "Verify an index",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := indexfile.Verify(command.Context(), args[0]); err != nil {
				return fmt.Errorf("verify index: %w", err)
			}
			if _, err := fmt.Fprintln(command.OutOrStdout(), "verified"); err != nil {
				return fmt.Errorf("write result: %w", err)
			}
			return nil
		},
	}
}

func newVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the DiskSeek version",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			command.Printf("version=%s\n", version)
		},
	}
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, version string) int {
	command := newRootCommand(version, stdout, stderr)
	command.SetContext(ctx)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		return 1
	}
	return 0
}
