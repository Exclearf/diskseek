package indexfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestReadRawPostingBlockHeaderAt(t *testing.T) {
	input := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	var buffer [rawPostingsPerBlock * rawPostingBytes]byte
	header, err := readRawPostingBlockHeaderAt(
		bytes.NewReader(input),
		termEntry{postingsOffset: fileHeaderBytes, postingsBytes: uint64(len(rawPostingBlockFixture))},
		fileHeaderBytes,
		2,
		2,
		buffer[:],
	)
	if err != nil {
		t.Fatal(err)
	}
	want := postingBlockHeader{lastDocumentID: index.DocumentID(1), payloadBytes: 16}
	if header != want {
		t.Fatalf("header = %+v, want %+v", header, want)
	}
}

func TestReadRawPostingBlockPayloadAt(t *testing.T) {
	input := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	var payload [rawPostingsPerBlock * rawPostingBytes]byte
	header, err := readRawPostingBlockHeaderAt(
		bytes.NewReader(input),
		termEntry{postingsOffset: fileHeaderBytes, postingsBytes: uint64(len(rawPostingBlockFixture))},
		fileHeaderBytes,
		2,
		2,
		payload[:],
	)
	if err != nil {
		t.Fatal(err)
	}

	var decoded [rawPostingsPerBlock]index.Posting
	if err := readRawPostingBlockPayloadAt(
		bytes.NewReader(input),
		fileHeaderBytes,
		header,
		[]uint32{1, 3},
		payload[:header.payloadBytes],
		decoded[:2],
	); err != nil {
		t.Fatal(err)
	}
	want := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 1, Frequency: 3},
	}
	if !slices.Equal(decoded[:2], want) {
		t.Fatalf("postings = %+v, want %+v", decoded[:2], want)
	}
}

func TestReadRawPostingBlockPayloadAtRejectsInvalidData(t *testing.T) {
	valid := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	zeroFrequency := slices.Clone(valid)
	binary.LittleEndian.PutUint32(
		zeroFrequency[fileHeaderBytes+rawPostingBlockHeaderBytes+4:],
		0,
	)
	tests := []struct {
		name            string
		input           []byte
		documentLengths []uint32
	}{
		{name: "short payload", input: valid[:len(valid)-1], documentLengths: []uint32{1, 3}},
		{name: "zero frequency", input: zeroFrequency, documentLengths: []uint32{1, 3}},
		{name: "frequency exceeds document length", input: valid, documentLengths: []uint32{1, 2}},
	}

	header := postingBlockHeader{lastDocumentID: 1, payloadBytes: 16}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload [rawPostingsPerBlock * rawPostingBytes]byte
			var decoded [rawPostingsPerBlock]index.Posting
			if err := readRawPostingBlockPayloadAt(
				bytes.NewReader(test.input),
				fileHeaderBytes,
				header,
				test.documentLengths,
				payload[:header.payloadBytes],
				decoded[:2],
			); err == nil {
				t.Fatal("readRawPostingBlockPayloadAt() error = nil")
			}
		})
	}
}

func TestReadRawPostingBlockHeaderAtChecksBounds(t *testing.T) {
	wrongPayloadLength := []byte(rawPostingBlockFixture)
	binary.LittleEndian.PutUint32(wrongPayloadLength[4:8], 8)
	tests := []struct {
		name         string
		term         termEntry
		blockOffset  uint64
		postingCount int
		input        []byte
		expectedRead bool
	}{
		{
			name:         "before term",
			term:         termEntry{postingsOffset: 9, postingsBytes: 24},
			blockOffset:  8,
			postingCount: 2,
		},
		{
			name:         "truncated header",
			term:         termEntry{postingsOffset: 8, postingsBytes: 7},
			blockOffset:  8,
			postingCount: 2,
		},
		{
			name:         "truncated payload",
			term:         termEntry{postingsOffset: 8, postingsBytes: 23},
			blockOffset:  8,
			postingCount: 2,
			expectedRead: true,
		},
		{
			name:         "wrong raw payload length",
			term:         termEntry{postingsOffset: 8, postingsBytes: 24},
			blockOffset:  8,
			postingCount: 2,
			input:        wrongPayloadLength,
			expectedRead: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			read := false
			var buffer [rawPostingsPerBlock * rawPostingBytes]byte
			input := readerAtFunc(func(data []byte, _ int64) (int, error) {
				read = true
				encoded := test.input
				if encoded == nil {
					encoded = []byte(rawPostingBlockFixture)
				}
				return copy(data, encoded), nil
			})
			if _, err := readRawPostingBlockHeaderAt(
				input,
				test.term,
				test.blockOffset,
				test.postingCount,
				2,
				buffer[:],
			); err == nil {
				t.Fatal("readRawPostingBlockHeaderAt() error = nil")
			}
			if read != test.expectedRead {
				t.Fatalf("header read = %t, want %t", read, test.expectedRead)
			}
		})
	}
}

func TestReadRawPostingBlockHeaderAtExactRead(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name    string
		read    int
		err     error
		wantErr error
	}{
		{name: "full with EOF", read: postingBlockHeaderBytes, err: io.EOF},
		{name: "short", read: postingBlockHeaderBytes - 1, err: io.EOF, wantErr: io.EOF},
		{name: "other error", read: postingBlockHeaderBytes, err: readErr, wantErr: readErr},
		{name: "joined EOF and other error", read: postingBlockHeaderBytes, err: errors.Join(io.EOF, readErr), wantErr: readErr},
	}

	term := termEntry{postingsOffset: 8, postingsBytes: 24}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buffer [rawPostingsPerBlock * rawPostingBytes]byte
			input := readerAtFunc(func(data []byte, _ int64) (int, error) {
				copy(data, rawPostingBlockFixture[:test.read])
				return test.read, test.err
			})
			_, err := readRawPostingBlockHeaderAt(input, term, 8, 2, 2, buffer[:])
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("readRawPostingBlockHeaderAt() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

type readerAtFunc func([]byte, int64) (int, error)

func (f readerAtFunc) ReadAt(data []byte, offset int64) (int, error) {
	return f(data, offset)
}
