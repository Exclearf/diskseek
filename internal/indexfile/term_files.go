package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

type TermFilesMetadata struct {
	Terms    FileMetadata
	Postings FileMetadata
	Codec    PostingsCodec
}

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

func WriteTermFiles(
	termOutput io.Writer,
	postingOutput io.Writer,
	nextTerm func() (string, uint64, error),
	nextPosting func() (index.Posting, error),
) (TermFilesMetadata, error) {
	terms, err := newFileWriter(termOutput, termsRole)
	if err != nil {
		return TermFilesMetadata{}, err
	}
	postings, err := newFileWriter(postingOutput, postingsRole)
	if err != nil {
		return TermFilesMetadata{}, err
	}

	if err := writeTermBodies(terms, postings, nextTerm, nextPosting); err != nil {
		return TermFilesMetadata{}, err
	}
	postingMetadata, err := postings.finish()
	if err != nil {
		return TermFilesMetadata{}, fmt.Errorf("finish postings: %w", err)
	}
	termMetadata, err := terms.finish()
	if err != nil {
		return TermFilesMetadata{}, fmt.Errorf("finish terms: %w", err)
	}

	return TermFilesMetadata{
		Terms:    termMetadata,
		Postings: postingMetadata,
		Codec:    PostingsCodecRaw,
	}, nil
}
