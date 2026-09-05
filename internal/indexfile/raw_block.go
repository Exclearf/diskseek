package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

const rawPostingBytes = 8

func writeRawPostingBlock(writer io.Writer, postings []index.Posting) error {
	payloadBytes, err := rawPostingBlockPayloadBytes(len(postings))
	if err != nil {
		return err
	}

	var encoded [postingBlockHeaderBytes + postingsPerBlock*rawPostingBytes]byte
	binary.LittleEndian.PutUint32(encoded[0:4], uint32(postings[len(postings)-1].DocumentID))
	binary.LittleEndian.PutUint32(encoded[4:8], uint32(payloadBytes))

	for position, posting := range postings {
		offset := postingBlockHeaderBytes + position*rawPostingBytes
		binary.LittleEndian.PutUint32(encoded[offset:offset+4], uint32(posting.DocumentID))
		binary.LittleEndian.PutUint32(encoded[offset+4:offset+8], posting.Frequency)
	}

	if _, err := writer.Write(encoded[:postingBlockHeaderBytes+payloadBytes]); err != nil {
		return fmt.Errorf("write raw posting block: %w", err)
	}
	return nil
}

func readRawPostingBlock(reader io.Reader, postingCount int, totalDocuments uint64) ([]index.Posting, error) {
	payloadBytes, err := rawPostingBlockPayloadBytes(postingCount)
	if err != nil {
		return nil, err
	}

	var encodedHeader [postingBlockHeaderBytes]byte
	if _, err := io.ReadFull(reader, encodedHeader[:]); err != nil {
		return nil, fmt.Errorf("read raw posting block header: %w", err)
	}
	header, err := decodePostingBlockHeader(encodedHeader[:], totalDocuments)
	if err != nil {
		return nil, err
	}
	if header.payloadBytes != uint32(payloadBytes) {
		return nil, errors.New("invalid raw posting block payload length")
	}

	var payload [postingsPerBlock * rawPostingBytes]byte
	if _, err := io.ReadFull(reader, payload[:payloadBytes]); err != nil {
		return nil, fmt.Errorf("read raw posting block payload: %w", err)
	}

	postings := make([]index.Posting, postingCount)
	if err := decodeRawPostingPayload(payload[:payloadBytes], postings); err != nil {
		return nil, err
	}
	if err := validatePostingBlock(postings, header); err != nil {
		return nil, err
	}
	return postings, nil
}

func decodeRawPostingPayload(payload []byte, postings []index.Posting) error {
	payloadBytes, err := rawPostingBlockPayloadBytes(len(postings))
	if err != nil {
		return err
	}
	if len(payload) != payloadBytes {
		return errors.New("invalid raw posting block payload length")
	}
	for position := range postings {
		offset := position * rawPostingBytes
		postings[position] = index.Posting{
			DocumentID: index.DocumentID(binary.LittleEndian.Uint32(payload[offset : offset+4])),
			Frequency:  binary.LittleEndian.Uint32(payload[offset+4 : offset+8]),
		}
	}
	return nil
}

func rawPostingBlockPayloadBytes(postingCount int) (int, error) {
	if postingCount < 1 || postingCount > postingsPerBlock {
		return 0, fmt.Errorf("raw posting block must contain 1 to %d postings", postingsPerBlock)
	}
	return postingCount * rawPostingBytes, nil
}
