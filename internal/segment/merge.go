package segment

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
)

const postingsPerCancellationCheck = runBufferBytes / encodedPostingBytes

type mergeTerm struct {
	term         string
	postingCount uint64
	runOrdinal   int
}

type mergeTermHeap []mergeTerm

func (h mergeTermHeap) Len() int { return len(h) }

func (h mergeTermHeap) Less(i, j int) bool {
	if h[i].term != h[j].term {
		return h[i].term < h[j].term
	}
	return h[i].runOrdinal < h[j].runOrdinal
}

func (h mergeTermHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeTermHeap) Push(value any) {
	*h = append(*h, value.(mergeTerm))
}

func (h *mergeTermHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	old[last] = mergeTerm{}
	*h = old[:last]
	return value
}

func mergeRunGroup(ctx context.Context, inputs []io.Reader, output io.WriteCloser) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(err, output.Close())
	}
	if len(inputs) < 2 {
		return errors.Join(errors.New("merge group must contain at least two runs"), output.Close())
	}

	readers := make([]*runReader, len(inputs))
	var nextDocumentID uint64
	for runOrdinal, source := range inputs {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, output.Close())
		}
		reader, err := newRunReader(source)
		if err != nil {
			return errors.Join(fmt.Errorf("read run %d header: %w", runOrdinal, err), output.Close())
		}
		if reader.header.documentCount == 0 {
			return errors.Join(fmt.Errorf("run %d has zero documents", runOrdinal), output.Close())
		}

		start := uint64(reader.header.firstDocumentID)
		if runOrdinal != 0 && start != nextDocumentID {
			return errors.Join(errors.New("run document intervals are not consecutive"), output.Close())
		}
		nextDocumentID = start + reader.header.documentCount
		readers[runOrdinal] = reader
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, output.Close())
	}

	firstDocumentID := readers[0].header.firstDocumentID
	writer, err := newRunWriter(output, runHeader{
		firstDocumentID: firstDocumentID,
		documentCount:   nextDocumentID - uint64(firstDocumentID),
	})
	if err != nil {
		return err
	}

	terms := make(mergeTermHeap, 0, len(readers))
	for runOrdinal, reader := range readers {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, writer.close())
		}
		term, postingCount, err := reader.nextTerm()
		if errors.Is(err, io.EOF) {
			continue
		}
		if err != nil {
			return errors.Join(fmt.Errorf("read term from run %d: %w", runOrdinal, err), writer.close())
		}
		terms = append(terms, mergeTerm{
			term:         term,
			postingCount: postingCount,
			runOrdinal:   runOrdinal,
		})
	}
	heap.Init(&terms)

	contributingRuns := make([]int, 0, len(readers))
	postingsUntilCancellationCheck := 0
	for terms.Len() != 0 {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, writer.close())
		}
		contributingRuns = contributingRuns[:0]
		term := terms[0].term
		var postingCount uint64
		for terms.Len() != 0 && terms[0].term == term {
			current := heap.Pop(&terms).(mergeTerm)
			contributingRuns = append(contributingRuns, current.runOrdinal)
			postingCount += current.postingCount
		}

		if err := writer.writeTerm(term, postingCount); err != nil {
			return writer.close()
		}
		for _, runOrdinal := range contributingRuns {
			reader := readers[runOrdinal]
			runPostingCount := reader.postingsRemaining
			for range runPostingCount {
				if postingsUntilCancellationCheck == 0 {
					if err := ctx.Err(); err != nil {
						return errors.Join(err, writer.close())
					}
					postingsUntilCancellationCheck = postingsPerCancellationCheck
				}
				postingsUntilCancellationCheck--

				posting, err := reader.nextPosting()
				if err != nil {
					return errors.Join(fmt.Errorf("read %q posting from run %d: %w", term, runOrdinal, err), writer.close())
				}
				if err := writer.writePosting(posting); err != nil {
					return writer.close()
				}
			}

			if err := ctx.Err(); err != nil {
				return errors.Join(err, writer.close())
			}
			nextTerm, nextPostingCount, err := reader.nextTerm()
			if errors.Is(err, io.EOF) {
				continue
			}
			if err != nil {
				return errors.Join(fmt.Errorf("read term from run %d: %w", runOrdinal, err), writer.close())
			}
			heap.Push(&terms, mergeTerm{
				term:         nextTerm,
				postingCount: nextPostingCount,
				runOrdinal:   runOrdinal,
			})
		}
	}

	if err := ctx.Err(); err != nil {
		return errors.Join(err, writer.close())
	}
	return writer.close()
}
