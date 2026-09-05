package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	rawPostingsPerBlock        = 128
	rawPostingBlockHeaderBytes = 8
	rawPostingBytes            = 8
)

func writeRawPostingBlock(writer io.Writer, postings []index.Posting) error {
	if len(postings) == 0 || len(postings) > rawPostingsPerBlock {
		return fmt.Errorf("raw posting block must contain 1 to %d postings", rawPostingsPerBlock)
	}

	var encoded [rawPostingBlockHeaderBytes + rawPostingsPerBlock*rawPostingBytes]byte
	binary.LittleEndian.PutUint32(encoded[0:4], uint32(postings[len(postings)-1].DocumentID))
	payloadBytes := len(postings) * rawPostingBytes
	binary.LittleEndian.PutUint32(encoded[4:8], uint32(payloadBytes))

	for position, posting := range postings {
		offset := rawPostingBlockHeaderBytes + position*rawPostingBytes
		binary.LittleEndian.PutUint32(encoded[offset:offset+4], uint32(posting.DocumentID))
		binary.LittleEndian.PutUint32(encoded[offset+4:offset+8], posting.Frequency)
	}

	if _, err := writer.Write(encoded[:rawPostingBlockHeaderBytes+payloadBytes]); err != nil {
		return fmt.Errorf("write raw posting block: %w", err)
	}
	return nil
}

func readRawPostingBlock(reader io.Reader, postingCount int, totalDocuments uint64) ([]index.Posting, error) {
	if postingCount < 1 || postingCount > rawPostingsPerBlock {
		return nil, fmt.Errorf("raw posting block must contain 1 to %d postings", rawPostingsPerBlock)
	}

	var header [rawPostingBlockHeaderBytes]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, fmt.Errorf("read raw posting block header: %w", err)
	}
	lastDocumentID := index.DocumentID(binary.LittleEndian.Uint32(header[0:4]))
	if uint64(lastDocumentID) >= totalDocuments {
		return nil, errors.New("raw posting block endpoint is outside the index")
	}

	expectedPayloadBytes := postingCount * rawPostingBytes
	if encoded := binary.LittleEndian.Uint32(header[4:8]); encoded != uint32(expectedPayloadBytes) {
		return nil, errors.New("invalid raw posting block payload length")
	}

	var payload [rawPostingsPerBlock * rawPostingBytes]byte
	if _, err := io.ReadFull(reader, payload[:expectedPayloadBytes]); err != nil {
		return nil, fmt.Errorf("read raw posting block payload: %w", err)
	}

	postings := make([]index.Posting, postingCount)
	for position := range postings {
		offset := position * rawPostingBytes
		posting := index.Posting{
			DocumentID: index.DocumentID(binary.LittleEndian.Uint32(payload[offset : offset+4])),
			Frequency:  binary.LittleEndian.Uint32(payload[offset+4 : offset+8]),
		}
		if posting.Frequency == 0 {
			return nil, errors.New("raw posting has zero frequency")
		}
		if position != 0 && posting.DocumentID <= postings[position-1].DocumentID {
			return nil, errors.New("raw posting document IDs are not strictly increasing")
		}
		postings[position] = posting
	}
	if postings[len(postings)-1].DocumentID != lastDocumentID {
		return nil, errors.New("raw posting block endpoint does not match its payload")
	}
	return postings, nil
}
