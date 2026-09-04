package segment

import (
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

func mergeRunPass(
	directory string,
	paths []string,
	fanIn int,
	passIndex int,
) ([]string, []mergeGroupStats, error) {
	groups, err := planMergePass(paths, fanIn)
	if err != nil {
		return nil, nil, err
	}

	successors := make([]string, len(groups))
	stats := make([]mergeGroupStats, len(groups))
	for _, group := range groups {
		path, inputBytes, outputBytes, err := mergeFileGroup(directory, passIndex, group)
		if err != nil {
			return nil, nil, fmt.Errorf("merge pass %d group %d: %w", passIndex, group.groupIndex, err)
		}
		successors[group.groupIndex] = path
		stats[group.groupIndex] = mergeGroupStats{
			passIndex:   passIndex,
			groupIndex:  group.groupIndex,
			inputCount:  len(group.inputPaths),
			inputBytes:  inputBytes,
			outputBytes: outputBytes,
		}
	}
	return successors, stats, nil
}

func mergeFileGroup(
	directory string,
	passIndex int,
	group mergeGroup,
) (outputPath string, inputBytes uint64, outputBytes uint64, err error) {
	if len(group.inputPaths) == 1 {
		info, err := os.Stat(group.inputPaths[0])
		if err != nil {
			return "", 0, 0, fmt.Errorf("stat carried run: %w", err)
		}
		size := uint64(info.Size())
		return group.inputPaths[0], size, size, nil
	}

	inputFiles := make([]*os.File, 0, len(group.inputPaths))
	defer func() {
		for _, input := range inputFiles {
			err = errors.Join(err, input.Close())
		}
	}()

	inputs := make([]io.Reader, 0, len(group.inputPaths))
	for _, path := range group.inputPaths {
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

	output, err := os.CreateTemp(directory, fmt.Sprintf("merge-%d-%d-*", passIndex, group.groupIndex))
	if err != nil {
		return "", 0, 0, fmt.Errorf("create successor run: %w", err)
	}
	outputPath = output.Name()
	if err := mergeRunGroup(inputs, output); err != nil {
		return "", 0, 0, err
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return "", 0, 0, fmt.Errorf("stat successor run: %w", err)
	}
	return outputPath, inputBytes, uint64(info.Size()), nil
}
