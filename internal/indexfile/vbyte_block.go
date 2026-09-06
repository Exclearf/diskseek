package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/Exclearf/diskseek/internal/index"
)

const maxVBytePostingPayloadBytes = postingsPerBlock * 2 * maxVByteUint32Bytes

func encodeVBytePostingPayload(destination []byte, postings []index.Posting) (int, error) {
	if _, _, err := vBytePostingPayloadBounds(len(postings)); err != nil {
		return 0, err
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
	minimumBytes, maximumBytes, err := vBytePostingPayloadBounds(len(postings))
	if err != nil {
		return err
	}
	if len(payload) < minimumBytes || len(payload) > maximumBytes {
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

		frequency, consumed, err := decodeVByteUint32(payload[offset:])
		if err != nil {
			return fmt.Errorf("decode variable-byte term frequency: %w", err)
		}
		offset += consumed
		if frequency == 0 {
			return errors.New("posting has zero frequency")
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

func writeVBytePostingBlock(writer io.Writer, encoded []byte, postings []index.Posting) (int, error) {
	payloadBytes, err := encodeVBytePostingPayload(encoded[postingBlockHeaderBytes:], postings)
	if err != nil {
		return 0, err
	}

	binary.LittleEndian.PutUint32(encoded[0:4], uint32(postings[len(postings)-1].DocumentID))
	binary.LittleEndian.PutUint32(encoded[4:8], uint32(payloadBytes))
	writtenBytes := postingBlockHeaderBytes + payloadBytes
	if _, err := writer.Write(encoded[:writtenBytes]); err != nil {
		return 0, fmt.Errorf("write variable-byte posting block: %w", err)
	}
	return writtenBytes, nil
}

func readVBytePostingBlock(
	reader io.Reader,
	postingCount int,
	totalDocuments uint64,
) ([]index.Posting, error) {
	minimumBytes, maximumBytes, err := vBytePostingPayloadBounds(postingCount)
	if err != nil {
		return nil, err
	}

	var encodedHeader [postingBlockHeaderBytes]byte
	if _, err := io.ReadFull(reader, encodedHeader[:]); err != nil {
		return nil, fmt.Errorf("read variable-byte posting block header: %w", err)
	}
	header, err := decodePostingBlockHeader(encodedHeader[:], totalDocuments)
	if err != nil {
		return nil, err
	}
	if header.payloadBytes < uint32(minimumBytes) || header.payloadBytes > uint32(maximumBytes) {
		return nil, errors.New("invalid variable-byte posting block payload length")
	}

	var payload [maxVBytePostingPayloadBytes]byte
	if _, err := io.ReadFull(reader, payload[:header.payloadBytes]); err != nil {
		return nil, fmt.Errorf("read variable-byte posting block payload: %w", err)
	}

	postings := make([]index.Posting, postingCount)
	if err := decodeVBytePostingPayload(payload[:header.payloadBytes], postings); err != nil {
		return nil, err
	}
	if err := validatePostingBlock(postings, header); err != nil {
		return nil, err
	}
	return postings, nil
}

func vBytePostingPayloadBounds(postingCount int) (int, int, error) {
	if postingCount < 1 || postingCount > postingsPerBlock {
		return 0, 0, fmt.Errorf("variable-byte payload must contain 1 to %d postings", postingsPerBlock)
	}
	minimumBytes := postingCount * 2
	return minimumBytes, minimumBytes * maxVByteUint32Bytes, nil
}
