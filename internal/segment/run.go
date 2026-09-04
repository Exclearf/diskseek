package segment

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	runMagic        = "DSKRUN01"
	runHeaderBytes  = 20
	maxRunTermBytes = 1 << 20
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

func readRunHeader(reader io.Reader) (runHeader, error) {
	var encoded [runHeaderBytes]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return runHeader{}, fmt.Errorf("read run header: %w", err)
	}
	if string(encoded[:8]) != runMagic {
		return runHeader{}, errors.New("invalid run magic")
	}

	header := runHeader{
		firstDocumentID: index.DocumentID(binary.LittleEndian.Uint32(encoded[8:12])),
		documentCount:   binary.LittleEndian.Uint64(encoded[12:20]),
	}
	if err := validateRunHeader(header); err != nil {
		return runHeader{}, err
	}
	return header, nil
}

func writeRunTermHeader(writer io.Writer, run runHeader, term string, postingCount uint64) error {
	if len(term) == 0 || len(term) > maxRunTermBytes || !utf8.ValidString(term) {
		return errors.New("invalid run term")
	}
	if postingCount == 0 || postingCount > run.documentCount {
		return errors.New("invalid run posting count")
	}

	var termLength [4]byte
	binary.LittleEndian.PutUint32(termLength[:], uint32(len(term)))
	if _, err := writer.Write(termLength[:]); err != nil {
		return fmt.Errorf("write run term length: %w", err)
	}
	if _, err := io.WriteString(writer, term); err != nil {
		return fmt.Errorf("write run term: %w", err)
	}

	var encodedCount [8]byte
	binary.LittleEndian.PutUint64(encodedCount[:], postingCount)
	if _, err := writer.Write(encodedCount[:]); err != nil {
		return fmt.Errorf("write run posting count: %w", err)
	}
	return nil
}

func readRunTermHeader(reader io.Reader, run runHeader) (string, uint64, error) {
	var encodedLength [4]byte
	if _, err := io.ReadFull(reader, encodedLength[:]); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", 0, fmt.Errorf("read run term length: %w", err)
	}

	termLength := binary.LittleEndian.Uint32(encodedLength[:])
	if termLength == 0 {
		var trailing [1]byte
		if _, err := io.ReadFull(reader, trailing[:]); errors.Is(err, io.EOF) {
			return "", 0, io.EOF
		} else if err != nil {
			return "", 0, fmt.Errorf("check run end: %w", err)
		}
		return "", 0, errors.New("run has trailing bytes")
	}
	if termLength > maxRunTermBytes {
		return "", 0, errors.New("invalid run term length")
	}

	termBytes := make([]byte, int(termLength))
	if _, err := io.ReadFull(reader, termBytes); err != nil {
		return "", 0, fmt.Errorf("read run term: %w", err)
	}
	if !utf8.Valid(termBytes) {
		return "", 0, errors.New("run term is not valid UTF-8")
	}

	var encodedCount [8]byte
	if _, err := io.ReadFull(reader, encodedCount[:]); err != nil {
		return "", 0, fmt.Errorf("read run posting count: %w", err)
	}
	postingCount := binary.LittleEndian.Uint64(encodedCount[:])
	if postingCount == 0 || postingCount > run.documentCount {
		return "", 0, errors.New("invalid run posting count")
	}
	return string(termBytes), postingCount, nil
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
