package indexfile

import (
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/corpus"
)

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
