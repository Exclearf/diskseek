package segment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

type mergeGroup struct {
	groupIndex int
	inputPaths []string
}

type mergeGroupStats struct {
	passIndex   int
	groupIndex  int
	inputCount  int
	inputBytes  uint64
	outputBytes uint64
}

func planMergePass(paths []string, fanIn int) ([]mergeGroup, error) {
	if fanIn < 2 {
		return nil, errors.New("merge fan-in must be at least two")
	}

	var groups []mergeGroup
	for inputPaths := range slices.Chunk(paths, fanIn) {
		groups = append(groups, mergeGroup{
			groupIndex: len(groups),
			inputPaths: inputPaths,
		})
	}
	return groups, nil
}

func mergeRuns(ctx context.Context, directory string, paths []string, fanIn, workers int) (string, []mergeGroupStats, error) {
	if fanIn < 2 {
		return "", nil, errors.New("merge fan-in must be at least two")
	}
	if workers < 1 {
		return "", nil, errors.New("merge worker count must be positive")
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	switch len(paths) {
	case 0:
		path, err := createEmptyRun(directory)
		if err != nil {
			return "", nil, fmt.Errorf("create empty run: %w", err)
		}
		return path, nil, nil
	case 1:
		if err := validateRunFile(ctx, paths[0]); err != nil {
			return "", nil, fmt.Errorf("validate sole run: %w", err)
		}
		return paths[0], nil, nil
	}

	currentPaths := paths
	var stats []mergeGroupStats
	for passIndex := 0; len(currentPaths) > 1; passIndex++ {
		successors, passStats, err := mergeRunPass(ctx, directory, currentPaths, fanIn, workers, passIndex)
		if err != nil {
			return "", nil, err
		}
		currentPaths = successors
		stats = append(stats, passStats...)
	}
	return currentPaths[0], stats, nil
}

func mergeRunPass(
	ctx context.Context,
	directory string,
	paths []string,
	fanIn int,
	workers int,
	passIndex int,
) ([]string, []mergeGroupStats, error) {
	groups, err := planMergePass(paths, fanIn)
	if err != nil {
		return nil, nil, err
	}

	results := runMergeGroups(ctx, groups, workers, func(ctx context.Context, group mergeGroup) (string, uint64, uint64, error) {
		return mergeFileGroup(ctx, directory, passIndex, group)
	})

	mergeErr := ctx.Err()
	for _, result := range results {
		if result.err != nil {
			mergeErr = errors.Join(mergeErr, fmt.Errorf("merge pass %d group %d: %w", passIndex, result.groupIndex, result.err))
		}
	}
	if mergeErr != nil {
		for _, result := range results {
			if result.err != nil || result.outputPath == "" || len(groups[result.groupIndex].inputPaths) == 1 {
				continue
			}
			mergeErr = errors.Join(mergeErr, os.Remove(result.outputPath))
		}
		return nil, nil, mergeErr
	}

	successors := make([]string, len(groups))
	stats := make([]mergeGroupStats, len(groups))
	for _, result := range results {
		successors[result.groupIndex] = result.outputPath
		stats[result.groupIndex] = mergeGroupStats{
			passIndex:   passIndex,
			groupIndex:  result.groupIndex,
			inputCount:  len(groups[result.groupIndex].inputPaths),
			inputBytes:  result.inputBytes,
			outputBytes: result.outputBytes,
		}
	}
	for _, group := range groups {
		if len(group.inputPaths) == 1 {
			continue
		}
		for _, path := range group.inputPaths {
			if err := os.Remove(path); err != nil {
				return nil, nil, fmt.Errorf("remove merged source: %w", err)
			}
		}
	}
	return successors, stats, nil
}

func mergeFileGroup(
	ctx context.Context,
	directory string,
	passIndex int,
	group mergeGroup,
) (outputPath string, inputBytes uint64, outputBytes uint64, err error) {
	if err := ctx.Err(); err != nil {
		return "", 0, 0, err
	}
	if len(group.inputPaths) == 1 {
		info, err := os.Stat(group.inputPaths[0])
		if err != nil {
			return "", 0, 0, fmt.Errorf("stat carried run: %w", err)
		}
		size := uint64(info.Size())
		return group.inputPaths[0], size, size, nil
	}

	inputFiles := make([]*os.File, 0, len(group.inputPaths))
	var ownedOutputPath string
	defer func() {
		for _, input := range inputFiles {
			err = errors.Join(err, input.Close())
		}
		if err != nil && ownedOutputPath != "" {
			err = errors.Join(err, os.Remove(ownedOutputPath))
		}
	}()

	inputs := make([]io.Reader, 0, len(group.inputPaths))
	for _, path := range group.inputPaths {
		if err := ctx.Err(); err != nil {
			return "", 0, 0, err
		}
		input, err := os.Open(path)
		if err != nil {
			return "", 0, 0, fmt.Errorf("open input run: %w", err)
		}
		inputFiles = append(inputFiles, input)
		inputs = append(inputs, input)

		info, err := input.Stat()
		if err != nil {
			return "", 0, 0, fmt.Errorf("stat input run: %w", err)
		}
		inputBytes += uint64(info.Size())
	}
	if err := ctx.Err(); err != nil {
		return "", 0, 0, err
	}

	output, err := os.CreateTemp(directory, fmt.Sprintf("merge-%d-%d-*", passIndex, group.groupIndex))
	if err != nil {
		return "", 0, 0, fmt.Errorf("create successor run: %w", err)
	}
	ownedOutputPath = output.Name()
	if err := mergeRunGroup(ctx, inputs, output); err != nil {
		return "", 0, 0, err
	}

	info, err := os.Stat(ownedOutputPath)
	if err != nil {
		return "", 0, 0, fmt.Errorf("stat successor run: %w", err)
	}
	return ownedOutputPath, inputBytes, uint64(info.Size()), nil
}

func createEmptyRun(directory string) (string, error) {
	output, err := os.CreateTemp(directory, "merge-empty-*")
	if err != nil {
		return "", err
	}
	path := output.Name()

	writer, err := newRunWriter(output, runHeader{})
	if err != nil {
		return "", err
	}
	if err := writer.close(); err != nil {
		return "", err
	}
	return path, nil
}

func validateRunFile(ctx context.Context, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, input.Close())
	}()
	return validateRun(ctx, input)
}

func validateRun(ctx context.Context, input io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reader, err := newRunReader(input)
	if err != nil {
		return err
	}
	postingsUntilCancellationCheck := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, postingCount, readErr := reader.nextTerm()
		if errors.Is(readErr, io.EOF) {
			return ctx.Err()
		}
		if readErr != nil {
			return readErr
		}
		for range postingCount {
			if postingsUntilCancellationCheck == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
				postingsUntilCancellationCheck = postingsPerCancellationCheck
			}
			postingsUntilCancellationCheck--

			if _, readErr := reader.nextPosting(); readErr != nil {
				return readErr
			}
		}
	}
}
