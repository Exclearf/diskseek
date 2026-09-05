package indexfile

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
)

func verifyPostingsFile(
	ctx context.Context,
	input io.Reader,
	size int64,
	codec PostingsCodec,
	terms map[string]termEntry,
	remainingTokenCounts []uint32,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reader, err := newFileReader(input, size, postingsRole)
	if err != nil {
		return err
	}

	orderedTerms := slices.Sorted(maps.Keys(terms))
	if err := ctx.Err(); err != nil {
		return err
	}
	totalDocuments := uint64(len(remainingTokenCounts))
	for _, term := range orderedTerms {
		entry := terms[term]
		visit := func(posting index.Posting) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			remaining := &remainingTokenCounts[posting.DocumentID]
			if posting.Frequency > *remaining {
				return fmt.Errorf("document %d term frequencies exceed its length", posting.DocumentID)
			}
			*remaining -= posting.Frequency
			return nil
		}

		var err error
		switch codec {
		case PostingsCodecRaw:
			err = readRawPostingList(
				reader,
				entry.documentFrequency,
				entry.postingsBytes,
				totalDocuments,
				visit,
			)
		case PostingsCodecVByte:
			err = readVBytePostingList(
				reader,
				entry.documentFrequency,
				entry.postingsBytes,
				totalDocuments,
				visit,
			)
		default:
			return fmt.Errorf("unsupported postings codec ID %d", codec)
		}
		if err != nil {
			return fmt.Errorf("verify %q postings: %w", term, err)
		}
	}
	if err := reader.finish(); err != nil {
		return fmt.Errorf("finish postings: %w", err)
	}
	for documentID, remaining := range remainingTokenCounts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if remaining != 0 {
			return fmt.Errorf("document %d has %d unaccounted tokens", documentID, remaining)
		}
	}
	return nil
}
