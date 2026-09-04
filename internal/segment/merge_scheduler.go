package segment

import (
	"context"
	"sync"
)

type mergeGroupResult struct {
	groupIndex  int
	outputPath  string
	inputBytes  uint64
	outputBytes uint64
	err         error
}

func runMergeGroups(
	ctx context.Context,
	groups []mergeGroup,
	workers int,
	merge func(context.Context, mergeGroup) (string, uint64, uint64, error),
) []mergeGroupResult {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan mergeGroup)
	completed := make(chan mergeGroupResult, len(groups))

	workerCount := min(workers, len(groups))
	var workerGroup sync.WaitGroup
	workerGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer workerGroup.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case group, ok := <-jobs:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					default:
					}

					path, inputBytes, outputBytes, err := merge(ctx, group)
					if err != nil {
						cancel()
					}
					completed <- mergeGroupResult{
						groupIndex:  group.groupIndex,
						outputPath:  path,
						inputBytes:  inputBytes,
						outputBytes: outputBytes,
						err:         err,
					}
					if err != nil {
						return
					}
				}
			}
		}()
	}

dispatch:
	for _, group := range groups {
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- group:
		}
	}
	close(jobs)
	workerGroup.Wait()
	close(completed)

	results := make([]mergeGroupResult, len(groups))
	for result := range completed {
		results[result.groupIndex] = result
	}
	return results
}
