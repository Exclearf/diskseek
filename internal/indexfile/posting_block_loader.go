package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

func readPostingBlockHeaderAt(
	input io.ReaderAt,
	term termEntry,
	blockOffset uint64,
	totalDocuments uint64,
	buffer []byte,
) (postingBlockHeader, error) {
	if blockOffset < term.postingsOffset {
		return postingBlockHeader{}, errors.New("posting block starts before its term range")
	}
	consumed := blockOffset - term.postingsOffset
	if consumed > term.postingsBytes || uint64(postingBlockHeaderBytes) > term.postingsBytes-consumed {
		return postingBlockHeader{}, errors.New("posting block header is outside its term range")
	}

	if len(buffer) < postingBlockHeaderBytes {
		return postingBlockHeader{}, errors.New("posting block buffer is too small")
	}
	encoded := buffer[:postingBlockHeaderBytes]
	if err := readAtExact(input, encoded, int64(blockOffset)); err != nil {
		return postingBlockHeader{}, fmt.Errorf("read posting block header: %w", err)
	}
	header, err := decodePostingBlockHeader(encoded, totalDocuments)
	if err != nil {
		return postingBlockHeader{}, err
	}
	remaining := term.postingsBytes - consumed - uint64(postingBlockHeaderBytes)
	if uint64(header.payloadBytes) > remaining {
		return postingBlockHeader{}, errors.New("posting block payload is outside its term range")
	}
	return header, nil
}

func readRawPostingBlockHeaderAt(
	input io.ReaderAt,
	term termEntry,
	blockOffset uint64,
	postingCount int,
	totalDocuments uint64,
	buffer []byte,
) (postingBlockHeader, error) {
	payloadBytes, err := rawPostingBlockPayloadBytes(postingCount)
	if err != nil {
		return postingBlockHeader{}, err
	}
	header, err := readPostingBlockHeaderAt(input, term, blockOffset, totalDocuments, buffer)
	if err != nil {
		return postingBlockHeader{}, err
	}
	if header.payloadBytes != uint32(payloadBytes) {
		return postingBlockHeader{}, errors.New("invalid raw posting block payload length")
	}
	return header, nil
}

func readRawPostingBlockPayloadAt(
	input io.ReaderAt,
	blockOffset uint64,
	header postingBlockHeader,
	documentLengths []uint32,
	payload []byte,
	postings []index.Posting,
) error {
	payloadBytes, err := rawPostingBlockPayloadBytes(len(postings))
	if err != nil {
		return err
	}
	if header.payloadBytes != uint32(payloadBytes) || len(payload) != payloadBytes {
		return errors.New("invalid raw posting block payload length")
	}
	if err := readAtExact(input, payload, int64(blockOffset+rawPostingBlockHeaderBytes)); err != nil {
		return fmt.Errorf("read raw posting block payload: %w", err)
	}
	if err := decodeRawPostingPayload(payload, postings); err != nil {
		return err
	}
	if err := validatePostingBlock(postings, header); err != nil {
		return err
	}
	for _, posting := range postings {
		if posting.Frequency > documentLengths[posting.DocumentID] {
			return fmt.Errorf("document %d term frequency exceeds its length", posting.DocumentID)
		}
	}
	return nil
}

func readAtExact(input io.ReaderAt, data []byte, offset int64) error {
	read, err := input.ReadAt(data, offset)
	if read != len(data) {
		if err != nil {
			return err
		}
		return io.ErrUnexpectedEOF
	}
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}
