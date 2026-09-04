package segment

import "sync"

type mergeGroupResult struct {
	groupIndex  int
	outputPath  string
	inputBytes  uint64
	outputBytes uint64
	err         error
}

func runMergeGroups(
	groups []mergeGroup,
	workers int,
	merge func(mergeGroup) (string, uint64, uint64, error),
) []mergeGroupResult {
	jobs := make(chan mergeGroup)
	completed := make(chan mergeGroupResult, len(groups))
	stop := make(chan struct{})
	var stopOnce sync.Once

	workerCount := min(workers, len(groups))
	var workerGroup sync.WaitGroup
	workerGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer workerGroup.Done()
			for {
				select {
				case <-stop:
					return
				case group, ok := <-jobs:
					if !ok {
						return
					}
					select {
					case <-stop:
						return
					default:
					}

					path, inputBytes, outputBytes, err := merge(group)
					if err != nil {
						stopOnce.Do(func() { close(stop) })
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
		case <-stop:
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
