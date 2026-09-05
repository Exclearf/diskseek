package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

const maximumPostingsPerList = uint64(1) << 32

func writeRawPostingList(
	writer io.Writer,
	postingCount uint64,
	nextPosting func() (index.Posting, error),
) (uint64, error) {
	if postingCount == 0 || postingCount > maximumPostingsPerList {
		return 0, errors.New("invalid raw posting-list count")
	}

	var block [rawPostingsPerBlock]index.Posting
	var writtenBytes uint64
	remaining := postingCount
	for remaining != 0 {
		currentCount := int(min(remaining, uint64(rawPostingsPerBlock)))
		for position := range currentCount {
			posting, err := nextPosting()
			if err != nil {
				postingNumber := postingCount - remaining + uint64(position)
				return 0, fmt.Errorf("read raw posting %d: %w", postingNumber, err)
			}
			block[position] = posting
		}

		if err := writeRawPostingBlock(writer, block[:currentCount]); err != nil {
			return 0, err
		}
		writtenBytes += uint64(rawPostingBlockHeaderBytes + currentCount*rawPostingBytes)
		remaining -= uint64(currentCount)
	}
	return writtenBytes, nil
}
