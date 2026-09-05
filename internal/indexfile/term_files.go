package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

func writeTermBodies(
	terms io.Writer,
	postings io.Writer,
	nextTerm func() (string, uint64, error),
	nextPosting func() (index.Posting, error),
) error {
	for {
		term, documentFrequency, err := nextTerm()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read term: %w", err)
		}

		postingsBytes, err := writeRawPostingList(postings, documentFrequency, nextPosting)
		if err != nil {
			return fmt.Errorf("write %q postings: %w", term, err)
		}
		if err := writeTermRecord(terms, termRecord{
			term:              term,
			documentFrequency: documentFrequency,
			postingsBytes:     postingsBytes,
		}); err != nil {
			return err
		}
	}
}
