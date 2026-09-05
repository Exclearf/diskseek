package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	documentOffsetBytes = 8
	maxDocumentCount    = uint64(1) << 32
)

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

func readExternalIDs(
	offsets io.Reader,
	data io.Reader,
	documentCount uint64,
	offsetBytes uint64,
	dataBytes uint64,
	visit func(string) error,
) error {
	if documentCount > maxDocumentCount {
		return errors.New("invalid document count")
	}
	expectedOffsetBytes := (documentCount + 1) * documentOffsetBytes
	if offsetBytes != expectedOffsetBytes {
		return errors.New("invalid document-offset body length")
	}

	previous, err := readDocumentOffset(offsets)
	if err != nil {
		return err
	}
	if previous != 0 {
		return errors.New("first document offset is not zero")
	}

	for documentID := uint64(0); documentID < documentCount; documentID++ {
		next, err := readDocumentOffset(offsets)
		if err != nil {
			return fmt.Errorf("read document %d end offset: %w", documentID, err)
		}
		if next <= previous {
			return errors.New("document offsets are not strictly increasing")
		}
		if next > dataBytes {
			return errors.New("document offset is outside the data body")
		}

		externalID, err := readExternalID(data, next-previous)
		if err != nil {
			return fmt.Errorf("read document %d external ID: %w", documentID, err)
		}
		if err := visit(externalID); err != nil {
			return fmt.Errorf("visit document %d external ID: %w", documentID, err)
		}
		previous = next
	}
	if previous != dataBytes {
		return errors.New("final document offset does not match the data body length")
	}
	return nil
}

func verifyExternalIDFiles(
	offsetInput io.Reader,
	offsetSize int64,
	dataInput io.Reader,
	dataSize int64,
	documentCount uint64,
) error {
	offsets, err := newFileReader(offsetInput, offsetSize, documentOffsetsRole)
	if err != nil {
		return err
	}
	data, err := newFileReader(dataInput, dataSize, documentDataRole)
	if err != nil {
		return err
	}

	if err := readExternalIDs(
		offsets,
		data,
		documentCount,
		uint64(offsets.body.N),
		uint64(data.body.N),
		func(string) error { return nil },
	); err != nil {
		return fmt.Errorf("verify external document IDs: %w", err)
	}
	if err := offsets.finish(); err != nil {
		return fmt.Errorf("finish document offsets: %w", err)
	}
	if err := data.finish(); err != nil {
		return fmt.Errorf("finish document data: %w", err)
	}
	return nil
}
