package indexfile

import (
	"encoding/binary"
	"fmt"
	"io"
)

const documentOffsetBytes = 8

func writeDocumentOffset(writer io.Writer, offset uint64) error {
	var encoded [documentOffsetBytes]byte
	binary.LittleEndian.PutUint64(encoded[:], offset)
	if _, err := writer.Write(encoded[:]); err != nil {
		return fmt.Errorf("write document offset: %w", err)
	}
	return nil
}

func readDocumentOffset(reader io.Reader) (uint64, error) {
	var encoded [documentOffsetBytes]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return 0, fmt.Errorf("read document offset: %w", err)
	}
	return binary.LittleEndian.Uint64(encoded[:]), nil
}
