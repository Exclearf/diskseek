package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func (i *Index) ExternalID(documentID index.DocumentID) (string, error) {
	if uint64(documentID) >= uint64(len(i.documentLengths)) {
		return "", errors.New("document ID is outside the index")
	}

	var encoded [2 * documentOffsetBytes]byte
	bodyOffset := uint64(documentID) * documentOffsetBytes
	if bodyOffset > i.documentOffsetsBodyBytes ||
		uint64(len(encoded)) > i.documentOffsetsBodyBytes-bodyOffset {
		return "", errors.New("document offset pair is outside the offsets body")
	}
	if err := readAtExact(i.documentOffsets, encoded[:], int64(fileHeaderBytes+bodyOffset)); err != nil {
		return "", fmt.Errorf("read document %d offsets: %w", documentID, err)
	}

	start := binary.LittleEndian.Uint64(encoded[:documentOffsetBytes])
	end := binary.LittleEndian.Uint64(encoded[documentOffsetBytes:])
	if end <= start {
		return "", errors.New("document offsets are not strictly increasing")
	}
	if end > i.documentDataBodyBytes {
		return "", errors.New("document offset is outside the data body")
	}

	length := end - start
	if length > corpus.MaxExternalIDBytes {
		return "", fmt.Errorf("read document %d external ID: invalid external document ID length", documentID)
	}
	externalID := make([]byte, int(length))
	if err := readAtExact(i.documentData, externalID, int64(fileHeaderBytes+start)); err != nil {
		return "", fmt.Errorf("read document %d external ID: %w", documentID, err)
	}
	if !utf8.Valid(externalID) {
		return "", fmt.Errorf("read document %d external ID: external document ID is not valid UTF-8", documentID)
	}
	return string(externalID), nil
}

func readExternalID(reader io.Reader, length uint64) (string, error) {
	if length == 0 || length > corpus.MaxExternalIDBytes {
		return "", errors.New("invalid external document ID length")
	}

	externalID := make([]byte, int(length))
	if _, err := io.ReadFull(reader, externalID); err != nil {
		return "", fmt.Errorf("read external document ID: %w", err)
	}
	if !utf8.Valid(externalID) {
		return "", errors.New("external document ID is not valid UTF-8")
	}
	return string(externalID), nil
}
