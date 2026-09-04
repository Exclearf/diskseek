package segment

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	runMagic        = "DSKRUN01"
	runHeaderBytes  = 20
	documentIDLimit = uint64(1) << 32
)

type runHeader struct {
	firstDocumentID index.DocumentID
	documentCount   uint64
}

func writeRunHeader(writer io.Writer, header runHeader) error {
	if err := validateRunHeader(header); err != nil {
		return err
	}

	var encoded [runHeaderBytes]byte
	copy(encoded[:8], runMagic)
	binary.LittleEndian.PutUint32(encoded[8:12], uint32(header.firstDocumentID))
	binary.LittleEndian.PutUint64(encoded[12:20], header.documentCount)
	if _, err := writer.Write(encoded[:]); err != nil {
		return fmt.Errorf("write run header: %w", err)
	}
	return nil
}

func validateRunHeader(header runHeader) error {
	if header.documentCount == 0 {
		if header.firstDocumentID != 0 {
			return errors.New("invalid empty run header")
		}
		return nil
	}

	start := uint64(header.firstDocumentID)
	if header.documentCount > documentIDLimit-start {
		return errors.New("run document interval overflows")
	}
	return nil
}

func documentInRun(header runHeader, documentID index.DocumentID) bool {
	value := uint64(documentID)
	start := uint64(header.firstDocumentID)
	return value >= start && value-start < header.documentCount
}
