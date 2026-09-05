package indexfile

import (
	"encoding/binary"
	"fmt"
	"io"
)

const documentLengthBytes = 4

type documentLengths struct {
	values             []uint32
	documentsWithTerms uint64
	totalLength        uint64
}

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

func readDocumentLengths(input io.Reader, size int64) (documentLengths, error) {
	reader, err := newFileReader(input, size, documentLengthsRole)
	if err != nil {
		return documentLengths{}, err
	}
	if reader.body.N%documentLengthBytes != 0 {
		return documentLengths{}, fmt.Errorf(
			"document-length body has %d bytes",
			reader.body.N,
		)
	}

	documentCount := uint64(reader.body.N / documentLengthBytes)
	if documentCount > maxDocumentCount {
		return documentLengths{}, fmt.Errorf("unsupported document count %d", documentCount)
	}

	result := documentLengths{values: make([]uint32, int(documentCount))}
	for documentID := range result.values {
		length, err := readDocumentLength(reader)
		if err != nil {
			return documentLengths{}, fmt.Errorf("read document %d length: %w", documentID, err)
		}
		result.values[documentID] = length
		if length != 0 {
			result.documentsWithTerms++
		}
		result.totalLength += uint64(length)
	}
	if err := reader.finish(); err != nil {
		return documentLengths{}, fmt.Errorf("finish document lengths: %w", err)
	}
	return result, nil
}
