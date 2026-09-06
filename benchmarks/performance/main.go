package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

const buildJobEnvironment = "DISKSEEK_BENCHMARK_BUILD_JOB"

type benchmarkConfig struct {
	CorpusPath         string      `json:"corpus"`
	OutputDirectory    string      `json:"output_directory"`
	IndexDirectory     string      `json:"index_directory"`
	TemporaryDirectory string      `json:"temporary_directory"`
	Build              buildConfig `json:"build"`
}

type buildConfig struct {
	Repetitions  int    `json:"repetitions"`
	FlushTarget  uint64 `json:"flush_target_bytes"`
	MergeFanIn   int    `json:"merge_fan_in"`
	MergeWorkers int    `json:"merge_workers"`
}

type buildJob struct {
	CorpusPath         string `json:"corpus"`
	TemporaryDirectory string `json:"temporary_directory"`
	Codec              string `json:"codec"`
	Repetition         int    `json:"repetition"`
	Destination        string `json:"destination"`
	FlushTarget        uint64 `json:"flush_target_bytes"`
	MergeFanIn         int    `json:"merge_fan_in"`
	MergeWorkers       int    `json:"merge_workers"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	status := 0
	if err := run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		status = 1
	}
	stop()
	os.Exit(status)
}

func run(ctx context.Context, arguments []string) error {
	if encodedJob, worker := os.LookupEnv(buildJobEnvironment); worker {
		return runBuildJob(ctx, encodedJob, os.Stdout)
	}
	if len(arguments) != 2 {
		return errors.New("usage: diskseek-benchmark CONFIG")
	}

	config, err := readConfig(arguments[1])
	if err != nil {
		return err
	}
	return runBuildPlan(ctx, config)
}

func readConfig(path string) (benchmarkConfig, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return benchmarkConfig{}, fmt.Errorf("read benchmark config: %w", err)
	}

	var config benchmarkConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return benchmarkConfig{}, fmt.Errorf("decode benchmark config: %w", err)
	}
	return config, nil
}

func runBuildPlan(ctx context.Context, config benchmarkConfig) error {
	for _, directory := range []string{
		config.OutputDirectory,
		config.IndexDirectory,
		config.TemporaryDirectory,
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create benchmark directory: %w", err)
		}
	}

	outputPath := filepath.Join(config.OutputDirectory, "builds.jsonl")
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create build observations: %w", err)
	}

	executable, err := os.Executable()
	if err != nil {
		return errors.Join(fmt.Errorf("find benchmark executable: %w", err), output.Close())
	}
	for repetition := 1; repetition <= config.Build.Repetitions; repetition++ {
		codecs := [2]string{"raw", "vbyte"}
		if repetition%2 == 0 {
			codecs[0], codecs[1] = codecs[1], codecs[0]
		}
		for _, codec := range codecs {
			job := buildJob{
				CorpusPath:         config.CorpusPath,
				TemporaryDirectory: config.TemporaryDirectory,
				Codec:              codec,
				Repetition:         repetition,
				Destination:        filepath.Join(config.IndexDirectory, fmt.Sprintf("%s-%d", codec, repetition)),
				FlushTarget:        config.Build.FlushTarget,
				MergeFanIn:         config.Build.MergeFanIn,
				MergeWorkers:       config.Build.MergeWorkers,
			}
			if err := runBuildProcess(ctx, executable, job, output); err != nil {
				return errors.Join(err, output.Close())
			}
		}
	}
	if err := output.Close(); err != nil {
		return err
	}
	return writeResults(config.OutputDirectory)
}

func runBuildProcess(ctx context.Context, executable string, job buildJob, output io.Writer) error {
	encodedJob, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode build job: %w", err)
	}

	command := exec.CommandContext(ctx, executable)
	command.Env = append(os.Environ(), buildJobEnvironment+"="+string(encodedJob))
	command.Stdout = output
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build %s repetition %d: %w", job.Codec, job.Repetition, err)
	}
	return nil
}
