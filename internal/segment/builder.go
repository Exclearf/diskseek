package segment

import (
	"errors"
	"io"

	"github.com/Exclearf/diskseek/internal/analyzer"
	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func buildRuns(
	records *corpus.TSVReader,
	flushTarget uint64,
	createOutput func() (io.WriteCloser, error),
) error {
	if flushTarget == 0 {
		return errors.New("segment flush target must be positive")
	}

	var documentCount uint64
	segment := newSegmentState(0)
	flush := func() error {
		output, err := createOutput()
		if err != nil {
			return err
		}
		if err := segment.writeRun(output); err != nil {
			return err
		}
		segment = newSegmentState(index.DocumentID(documentCount))
		return nil
	}

	for {
		record, err := records.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if documentCount == documentIDLimit {
			return errors.New("document count exceeds supported limit")
		}

		tokens, err := analyzer.Analyze(record.Text)
		if err != nil {
			return err
		}
		accountedBytes := segment.addDocument(tokens)
		documentCount++

		if accountedBytes >= flushTarget {
			if err := flush(); err != nil {
				return err
			}
		}
	}

	if segment.documentCount != 0 {
		return flush()
	}
	return nil
}
