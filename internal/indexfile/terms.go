package indexfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	maxTermBytes          = uint32(1 << 20)
	termRecordHeaderBytes = 20
)

type termRecord struct {
	term              string
	documentFrequency uint64
	postingsBytes     uint64
}

type termEntry struct {
	documentFrequency uint64
	postingsOffset    uint64
	postingsBytes     uint64
}

func writeTermRecord(writer io.Writer, record termRecord) error {
	var header [termRecordHeaderBytes]byte
	binary.LittleEndian.PutUint32(header[:4], uint32(len(record.term)))
	binary.LittleEndian.PutUint64(header[4:12], record.documentFrequency)
	binary.LittleEndian.PutUint64(header[12:20], record.postingsBytes)
	if _, err := writer.Write(header[:]); err != nil {
		return fmt.Errorf("write term record header: %w", err)
	}
	if _, err := io.WriteString(writer, record.term); err != nil {
		return fmt.Errorf("write term: %w", err)
	}
	return nil
}

func readTermRecord(
	reader io.Reader,
	remainingBytes uint64,
	documentsWithTerms uint64,
) (termRecord, error) {
	if remainingBytes < termRecordHeaderBytes {
		return termRecord{}, errors.New("term record is truncated")
	}

	var header [termRecordHeaderBytes]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return termRecord{}, fmt.Errorf("read term record header: %w", err)
	}
	termLength := binary.LittleEndian.Uint32(header[:4])
	if termLength == 0 || termLength > maxTermBytes {
		return termRecord{}, errors.New("invalid term length")
	}
	if termRecordHeaderBytes+uint64(termLength) > remainingBytes {
		return termRecord{}, errors.New("term record crosses the file body")
	}
	documentFrequency := binary.LittleEndian.Uint64(header[4:12])
	if documentFrequency == 0 || documentFrequency > documentsWithTerms {
		return termRecord{}, errors.New("invalid document frequency")
	}
	postingsBytes := binary.LittleEndian.Uint64(header[12:20])
	if postingsBytes == 0 {
		return termRecord{}, errors.New("invalid postings length")
	}

	termBytes := make([]byte, int(termLength))
	if _, err := io.ReadFull(reader, termBytes); err != nil {
		return termRecord{}, fmt.Errorf("read term: %w", err)
	}
	if !utf8.Valid(termBytes) {
		return termRecord{}, errors.New("term is not valid UTF-8")
	}
	return termRecord{
		term:              string(termBytes),
		documentFrequency: documentFrequency,
		postingsBytes:     postingsBytes,
	}, nil
}

func readTermFile(
	input io.Reader,
	size int64,
	postingsBodyBytes uint64,
	documentsWithTerms uint64,
) (map[string]termEntry, error) {
	reader, err := newFileReader(input, size, termsRole)
	if err != nil {
		return nil, err
	}
	terms, err := readTerms(
		reader,
		uint64(reader.body.N),
		postingsBodyBytes,
		documentsWithTerms,
	)
	if err != nil {
		return nil, err
	}
	if err := reader.finish(); err != nil {
		return nil, fmt.Errorf("finish terms: %w", err)
	}
	return terms, nil
}

func readTerms(
	reader io.Reader,
	termBodyBytes uint64,
	postingsBodyBytes uint64,
	documentsWithTerms uint64,
) (map[string]termEntry, error) {
	terms := make(map[string]termEntry)
	remaining := termBodyBytes
	var previousTerm string
	var consumedPostings uint64

	for remaining != 0 {
		record, err := readTermRecord(reader, remaining, documentsWithTerms)
		if err != nil {
			return nil, fmt.Errorf("read term %d: %w", len(terms), err)
		}
		recordBytes := termRecordHeaderBytes + uint64(len(record.term))
		remaining -= recordBytes

		if previousTerm != "" && record.term <= previousTerm {
			return nil, errors.New("terms are not strictly increasing")
		}
		if record.postingsBytes > postingsBodyBytes-consumedPostings {
			return nil, errors.New("term postings range is outside the postings body")
		}

		terms[record.term] = termEntry{
			documentFrequency: record.documentFrequency,
			postingsOffset:    fileHeaderBytes + consumedPostings,
			postingsBytes:     record.postingsBytes,
		}
		consumedPostings += record.postingsBytes
		previousTerm = record.term
	}
	if consumedPostings != postingsBodyBytes {
		return nil, errors.New("term ranges do not cover the postings body")
	}
	return terms, nil
}
