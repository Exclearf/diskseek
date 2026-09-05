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
	postingCount int,
	totalDocuments uint64,
	codec PostingsCodec,
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

	switch codec {
	case PostingsCodecRaw:
		payloadBytes, err := rawPostingBlockPayloadBytes(postingCount)
		if err != nil {
			return postingBlockHeader{}, err
		}
		if header.payloadBytes != uint32(payloadBytes) {
			return postingBlockHeader{}, errors.New("invalid raw posting block payload length")
		}
	case PostingsCodecVByte:
		minimumBytes, maximumBytes, err := vBytePostingPayloadBounds(postingCount)
		if err != nil {
			return postingBlockHeader{}, err
		}
		if header.payloadBytes < uint32(minimumBytes) || header.payloadBytes > uint32(maximumBytes) {
			return postingBlockHeader{}, errors.New("invalid variable-byte posting block payload length")
		}
	default:
		return postingBlockHeader{}, fmt.Errorf("unsupported postings codec ID %d", codec)
	}
	return header, nil
}

func readPostingBlockPayloadAt(
	input io.ReaderAt,
	blockOffset uint64,
	header postingBlockHeader,
	codec PostingsCodec,
	documentLengths []uint32,
	payload []byte,
	postings []index.Posting,
) error {
	if uint32(len(payload)) != header.payloadBytes {
		return errors.New("posting block payload length does not match its header")
	}
	if err := readAtExact(input, payload, int64(blockOffset+postingBlockHeaderBytes)); err != nil {
		return fmt.Errorf("read posting block payload: %w", err)
	}
	switch codec {
	case PostingsCodecRaw:
		if err := decodeRawPostingPayload(payload, postings); err != nil {
			return err
		}
	case PostingsCodecVByte:
		if err := decodeVBytePostingPayload(payload, postings); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported postings codec ID %d", codec)
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
