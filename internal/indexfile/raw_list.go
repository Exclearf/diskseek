package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

func writeRawPostingList(
	writer io.Writer,
	encoded []byte,
	postingCount uint64,
	nextPosting func() (index.Posting, error),
) (uint64, error) {
	writtenBytes, err := rawPostingListBytes(postingCount)
	if err != nil {
		return 0, err
	}

	var block [postingsPerBlock]index.Posting
	remaining := postingCount
	for remaining != 0 {
		currentCount := int(min(remaining, uint64(postingsPerBlock)))
		for position := range currentCount {
			posting, err := nextPosting()
			if err != nil {
				postingNumber := postingCount - remaining + uint64(position)
				return 0, fmt.Errorf("read raw posting %d: %w", postingNumber, err)
			}
			block[position] = posting
		}

		if err := writeRawPostingBlock(writer, encoded, block[:currentCount]); err != nil {
			return 0, err
		}
		remaining -= uint64(currentCount)
	}
	return writtenBytes, nil
}

func readRawPostingList(
	reader io.Reader,
	postingCount uint64,
	postingBytes uint64,
	totalDocuments uint64,
	visitPosting func(index.Posting) error,
) error {
	expectedBytes, err := rawPostingListBytes(postingCount)
	if err != nil {
		return err
	}
	if postingBytes != expectedBytes {
		return errors.New("invalid raw posting-list byte length")
	}

	remaining := postingCount
	var previousDocumentID index.DocumentID
	for blockIndex := 0; remaining != 0; blockIndex++ {
		currentCount := int(min(remaining, uint64(postingsPerBlock)))
		block, err := readRawPostingBlock(reader, currentCount, totalDocuments)
		if err != nil {
			return fmt.Errorf("read raw posting block %d: %w", blockIndex, err)
		}
		if blockIndex != 0 && block[0].DocumentID <= previousDocumentID {
			return errors.New("raw posting document IDs are not strictly increasing across blocks")
		}
		for _, posting := range block {
			if err := visitPosting(posting); err != nil {
				return fmt.Errorf("visit raw posting: %w", err)
			}
		}
		previousDocumentID = block[len(block)-1].DocumentID
		remaining -= uint64(currentCount)
	}
	return nil
}

func rawPostingListBytes(postingCount uint64) (uint64, error) {
	if postingCount == 0 || postingCount > maxPostingsPerList {
		return 0, errors.New("invalid raw posting-list count")
	}
	blockCount := (postingCount-1)/uint64(postingsPerBlock) + 1
	return postingCount*uint64(rawPostingBytes) + blockCount*uint64(postingBlockHeaderBytes), nil
}
