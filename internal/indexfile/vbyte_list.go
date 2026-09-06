package indexfile

import (
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/Exclearf/diskseek/internal/index"
)

func writeVBytePostingList(
	writer io.Writer,
	encoded []byte,
	postingCount uint64,
	nextPosting func() (index.Posting, error),
) (uint64, error) {
	if postingCount == 0 || postingCount > maxPostingsPerList {
		return 0, errors.New("invalid variable-byte posting-list count")
	}

	var block [postingsPerBlock]index.Posting
	remaining := postingCount
	var writtenBytes uint64
	for remaining != 0 {
		currentCount := int(min(remaining, uint64(postingsPerBlock)))
		for position := range currentCount {
			posting, err := nextPosting()
			if err != nil {
				postingNumber := postingCount - remaining + uint64(position)
				return 0, fmt.Errorf("read variable-byte posting %d: %w", postingNumber, err)
			}
			block[position] = posting
		}

		blockBytes, err := writeVBytePostingBlock(writer, encoded, block[:currentCount])
		if err != nil {
			return 0, err
		}
		writtenBytes += uint64(blockBytes)
		remaining -= uint64(currentCount)
	}
	return writtenBytes, nil
}

func readVBytePostingList(
	reader io.Reader,
	postingCount uint64,
	postingBytes uint64,
	totalDocuments uint64,
	visitPosting func(index.Posting) error,
) error {
	if postingCount == 0 || postingCount > maxPostingsPerList {
		return errors.New("invalid variable-byte posting-list count")
	}
	if postingBytes > math.MaxInt64 {
		return errors.New("variable-byte posting list is too large")
	}

	limited := io.LimitedReader{R: reader, N: int64(postingBytes)}
	remaining := postingCount
	var previousDocumentID index.DocumentID
	for blockIndex := 0; remaining != 0; blockIndex++ {
		currentCount := int(min(remaining, uint64(postingsPerBlock)))
		block, err := readVBytePostingBlock(&limited, currentCount, totalDocuments)
		if err != nil {
			return fmt.Errorf("read variable-byte posting block %d: %w", blockIndex, err)
		}
		if blockIndex != 0 && block[0].DocumentID <= previousDocumentID {
			return errors.New("variable-byte posting document IDs are not strictly increasing across blocks")
		}
		for _, posting := range block {
			if err := visitPosting(posting); err != nil {
				return fmt.Errorf("visit variable-byte posting: %w", err)
			}
		}
		previousDocumentID = block[len(block)-1].DocumentID
		remaining -= uint64(currentCount)
	}
	if limited.N != 0 {
		return errors.New("invalid variable-byte posting-list byte length")
	}
	return nil
}
