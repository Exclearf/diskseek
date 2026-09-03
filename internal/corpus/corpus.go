package corpus

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"unicode/utf8"
)

const (
	MaxRecordBytes     = 1 << 20
	MaxExternalIDBytes = 1 << 10
)

var (
	ErrMalformedRecord    = errors.New("malformed corpus record")
	ErrRecordTooLarge     = errors.New("corpus record too large")
	ErrExternalIDTooLarge = errors.New("external ID too large")
	ErrEmptyExternalID    = errors.New("empty external ID")
	ErrInvalidUTF8        = errors.New("invalid UTF-8")
)

type Record struct {
	ExternalID string
	Text       string
}

type TSVReader struct {
	scanner *bufio.Scanner
}

func NewTSVReader(input io.Reader) *TSVReader {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(nil, MaxRecordBytes+2)
	return &TSVReader{scanner: scanner}
}

func (r *TSVReader) Next() (Record, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				return Record{}, ErrRecordTooLarge
			}
			return Record{}, err
		}
		return Record{}, io.EOF
	}

	line := r.scanner.Bytes()
	if len(line) > MaxRecordBytes {
		return Record{}, ErrRecordTooLarge
	}
	if !utf8.Valid(line) {
		return Record{}, ErrInvalidUTF8
	}

	tab := bytes.IndexByte(line, '\t')
	if tab == -1 {
		return Record{}, ErrMalformedRecord
	}
	if tab == 0 {
		return Record{}, ErrEmptyExternalID
	}
	if tab > MaxExternalIDBytes {
		return Record{}, ErrExternalIDTooLarge
	}

	return Record{
		ExternalID: string(line[:tab]),
		Text:       string(line[tab+1:]),
	}, nil
}
