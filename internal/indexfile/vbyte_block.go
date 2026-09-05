package indexfile

import (
	"errors"
	"fmt"
	"math"

	"github.com/Exclearf/diskseek/internal/index"
)

const maxVBytePostingPayloadBytes = postingsPerBlock * 2 * maxVByteUint32Bytes

func encodeVBytePostingPayload(destination []byte, postings []index.Posting) (int, error) {
	if len(postings) < 1 || len(postings) > postingsPerBlock {
		return 0, fmt.Errorf("variable-byte payload must contain 1 to %d postings", postingsPerBlock)
	}
	if err := validatePostingBlock(postings, postingBlockHeader{
		lastDocumentID: postings[len(postings)-1].DocumentID,
	}); err != nil {
		return 0, err
	}

	written := 0
	var previousDocumentID index.DocumentID
	for position, posting := range postings {
		documentValue := uint32(posting.DocumentID)
		if position != 0 {
			documentValue -= uint32(previousDocumentID)
		}
		written += encodeVByteUint32(destination[written:], documentValue)
		written += encodeVByteUint32(destination[written:], posting.Frequency)
		previousDocumentID = posting.DocumentID
	}
	return written, nil
}

func decodeVBytePostingPayload(payload []byte, postings []index.Posting) error {
	postingCount := len(postings)
	if postingCount < 1 || postingCount > postingsPerBlock {
		return fmt.Errorf("variable-byte payload must contain 1 to %d postings", postingsPerBlock)
	}
	if len(payload) < postingCount*2 || len(payload) > postingCount*2*maxVByteUint32Bytes {
		return errors.New("invalid variable-byte posting payload length")
	}

	offset := 0
	var previousDocumentID index.DocumentID
	for position := range postings {
		documentValue, consumed, err := decodeVByteUint32(payload[offset:])
		if err != nil {
			return fmt.Errorf("decode variable-byte document value: %w", err)
		}
		offset += consumed

		frequency, consumed, err := decodeVByteUint32(payload[offset:])
		if err != nil {
			return fmt.Errorf("decode variable-byte term frequency: %w", err)
		}
		offset += consumed
		if frequency == 0 {
			return errors.New("posting has zero frequency")
		}

		documentID := index.DocumentID(documentValue)
		if position != 0 {
			if documentValue == 0 {
				return errors.New("posting has zero document gap")
			}
			nextDocumentID := uint64(previousDocumentID) + uint64(documentValue)
			if nextDocumentID > math.MaxUint32 {
				return errors.New("posting document ID overflows uint32")
			}
			documentID = index.DocumentID(nextDocumentID)
		}

		postings[position] = index.Posting{
			DocumentID: documentID,
			Frequency:  frequency,
		}
		previousDocumentID = documentID
	}
	if offset != len(payload) {
		return errors.New("variable-byte posting payload has trailing bytes")
	}
	return nil
}
