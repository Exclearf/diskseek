package indexfile

import (
	"encoding/binary"
	"fmt"
	"io"
)

const documentLengthBytes = 4

func writeDocumentLength(writer io.Writer, length uint32) error {
	var encoded [documentLengthBytes]byte
	binary.LittleEndian.PutUint32(encoded[:], length)
	if _, err := writer.Write(encoded[:]); err != nil {
		return fmt.Errorf("write document length: %w", err)
	}
	return nil
}

func readDocumentLength(reader io.Reader) (uint32, error) {
	var encoded [documentLengthBytes]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return 0, fmt.Errorf("read document length: %w", err)
	}
	return binary.LittleEndian.Uint32(encoded[:]), nil
}
