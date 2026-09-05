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

func TestReadPostingBlockHeaderAtRaw(t *testing.T) {
	input := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	var buffer [postingsPerBlock * rawPostingBytes]byte
	header, err := readPostingBlockHeaderAt(
		bytes.NewReader(input),
		termEntry{postingsOffset: fileHeaderBytes, postingsBytes: uint64(len(rawPostingBlockFixture))},
		fileHeaderBytes,
		2,
		2,
		PostingsCodecRaw,
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

func TestReadPostingBlockPayloadAtRaw(t *testing.T) {
	input := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	var payload [postingsPerBlock * rawPostingBytes]byte
	header, err := readPostingBlockHeaderAt(
		bytes.NewReader(input),
		termEntry{postingsOffset: fileHeaderBytes, postingsBytes: uint64(len(rawPostingBlockFixture))},
		fileHeaderBytes,
		2,
		2,
		PostingsCodecRaw,
		payload[:],
	)
	if err != nil {
		t.Fatal(err)
	}

	var decoded [postingsPerBlock]index.Posting
	if err := readPostingBlockPayloadAt(
		bytes.NewReader(input),
		fileHeaderBytes,
		header,
		PostingsCodecRaw,
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

func TestReadPostingBlockPayloadAtRejectsInvalidRawData(t *testing.T) {
	valid := append(make([]byte, fileHeaderBytes), rawPostingBlockFixture...)
	zeroFrequency := slices.Clone(valid)
	binary.LittleEndian.PutUint32(
		zeroFrequency[fileHeaderBytes+postingBlockHeaderBytes+4:],
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
			var payload [postingsPerBlock * rawPostingBytes]byte
			var decoded [postingsPerBlock]index.Posting
			if err := readPostingBlockPayloadAt(
				bytes.NewReader(test.input),
				fileHeaderBytes,
				header,
				PostingsCodecRaw,
				test.documentLengths,
				payload[:header.payloadBytes],
				decoded[:2],
			); err == nil {
				t.Fatal("readPostingBlockPayloadAt() error = nil")
			}
		})
	}
}

func TestReadPostingBlockHeaderAtChecksRawBounds(t *testing.T) {
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
			var buffer [postingsPerBlock * rawPostingBytes]byte
			input := readerAtFunc(func(data []byte, _ int64) (int, error) {
				read = true
				encoded := test.input
				if encoded == nil {
					encoded = []byte(rawPostingBlockFixture)
				}
				return copy(data, encoded), nil
			})
			if _, err := readPostingBlockHeaderAt(
				input,
				test.term,
				test.blockOffset,
				test.postingCount,
				2,
				PostingsCodecRaw,
				buffer[:],
			); err == nil {
				t.Fatal("readPostingBlockHeaderAt() error = nil")
			}
			if read != test.expectedRead {
				t.Fatalf("header read = %t, want %t", read, test.expectedRead)
			}
		})
	}
}

func TestReadPostingBlockHeaderAtChecksVBytePayloadBounds(t *testing.T) {
	for _, payloadBytes := range []uint32{3, 21} {
		var encoded [postingBlockHeaderBytes]byte
		binary.LittleEndian.PutUint32(encoded[0:4], 1)
		binary.LittleEndian.PutUint32(encoded[4:8], payloadBytes)
		input := append(make([]byte, fileHeaderBytes), encoded[:]...)

		var buffer [postingBlockHeaderBytes]byte
		if _, err := readPostingBlockHeaderAt(
			bytes.NewReader(input),
			termEntry{
				postingsOffset: fileHeaderBytes,
				postingsBytes:  postingBlockHeaderBytes + uint64(payloadBytes),
			},
			fileHeaderBytes,
			2,
			2,
			PostingsCodecVByte,
			buffer[:],
		); err == nil {
			t.Fatalf("readPostingBlockHeaderAt() error = nil for %d payload bytes", payloadBytes)
		}
	}
}

func TestReadPostingBlockHeaderAtExactRead(t *testing.T) {
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
			var buffer [postingsPerBlock * rawPostingBytes]byte
			input := readerAtFunc(func(data []byte, _ int64) (int, error) {
				copy(data, rawPostingBlockFixture[:test.read])
				return test.read, test.err
			})
			_, err := readPostingBlockHeaderAt(input, term, 8, 2, 2, PostingsCodecRaw, buffer[:])
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("readPostingBlockHeaderAt() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

type readerAtFunc func([]byte, int64) (int, error)

func (f readerAtFunc) ReadAt(data []byte, offset int64) (int, error) {
	return f(data, offset)
}
