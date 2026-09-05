package indexfile

import (
	"encoding/binary"
	"fmt"
	"io"
)

func writeTermRecord(writer io.Writer, term string, documentFrequency, postingsBytes uint64) error {
	var termLength [4]byte
	binary.LittleEndian.PutUint32(termLength[:], uint32(len(term)))
	if _, err := writer.Write(termLength[:]); err != nil {
		return fmt.Errorf("write term length: %w", err)
	}
	if _, err := io.WriteString(writer, term); err != nil {
		return fmt.Errorf("write term: %w", err)
	}

	var metadata [16]byte
	binary.LittleEndian.PutUint64(metadata[:8], documentFrequency)
	binary.LittleEndian.PutUint64(metadata[8:], postingsBytes)
	if _, err := writer.Write(metadata[:]); err != nil {
		return fmt.Errorf("write term metadata: %w", err)
	}
	return nil
}
