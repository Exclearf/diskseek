package indexfile

import (
	"encoding/binary"
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
