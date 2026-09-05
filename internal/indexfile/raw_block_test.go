package indexfile

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

const rawPostingBlockFixture = "\x01\x00\x00\x00" +
	"\x10\x00\x00\x00" +
	"\x00\x00\x00\x00\x01\x00\x00\x00" +
	"\x01\x00\x00\x00\x03\x00\x00\x00"

func TestWriteRawPostingBlockBytes(t *testing.T) {
	postings := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 1, Frequency: 3},
	}
	var encoded bytes.Buffer
	if err := writeRawPostingBlock(&encoded, postings); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), []byte(rawPostingBlockFixture)) {
		t.Fatalf("block = % x, want % x", encoded.Bytes(), rawPostingBlockFixture)
	}
}

func TestRawPostingBlockCount(t *testing.T) {
	postings := make([]index.Posting, postingsPerBlock)
	for documentID := range postings {
		postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
	}

	var encoded bytes.Buffer
	if err := writeRawPostingBlock(&encoded, postings); err != nil {
		t.Fatal(err)
	}
	if got, want := encoded.Len(), postingBlockHeaderBytes+postingsPerBlock*rawPostingBytes; got != want {
		t.Fatalf("block length = %d, want %d", got, want)
	}
	if got := binary.LittleEndian.Uint32(encoded.Bytes()[4:8]); got != postingsPerBlock*rawPostingBytes {
		t.Fatalf("payload length = %d, want %d", got, postingsPerBlock*rawPostingBytes)
	}
	decoded, err := readRawPostingBlock(bytes.NewReader(encoded.Bytes()), len(postings), uint64(len(postings)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(decoded, postings) {
		t.Fatal("maximum-size block did not round-trip")
	}

	if err := writeRawPostingBlock(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("writeRawPostingBlock() error = nil for empty block")
	}
	if err := writeRawPostingBlock(&bytes.Buffer{}, make([]index.Posting, postingsPerBlock+1)); err == nil {
		t.Fatal("writeRawPostingBlock() error = nil for oversized block")
	}
}

func TestReadRawPostingBlock(t *testing.T) {
	want := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 1, Frequency: 3},
	}
	got, err := readRawPostingBlock(bytes.NewBufferString(rawPostingBlockFixture), len(want), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("postings = %+v, want %+v", got, want)
	}
}

func TestReadRawPostingBlockRejectsInvalidData(t *testing.T) {
	wrongPayloadLength := []byte(rawPostingBlockFixture)
	binary.LittleEndian.PutUint32(wrongPayloadLength[4:8], 8)

	zeroFrequency := []byte(rawPostingBlockFixture)
	binary.LittleEndian.PutUint32(zeroFrequency[12:16], 0)

	notIncreasing := []byte(rawPostingBlockFixture)
	binary.LittleEndian.PutUint32(notIncreasing[0:4], 0)
	binary.LittleEndian.PutUint32(notIncreasing[8:12], 1)
	binary.LittleEndian.PutUint32(notIncreasing[16:20], 0)

	wrongLastDocumentID := []byte(rawPostingBlockFixture)
	binary.LittleEndian.PutUint32(wrongLastDocumentID[0:4], 0)

	tests := []struct {
		name           string
		data           []byte
		postingCount   int
		totalDocuments uint64
	}{
		{name: "zero count", postingCount: 0, totalDocuments: 2},
		{name: "oversized count", postingCount: postingsPerBlock + 1, totalDocuments: 2},
		{name: "wrong payload length", data: wrongPayloadLength, postingCount: 2, totalDocuments: 2},
		{name: "out-of-range endpoint", data: []byte(rawPostingBlockFixture), postingCount: 2, totalDocuments: 1},
		{name: "zero frequency", data: zeroFrequency, postingCount: 2, totalDocuments: 2},
		{name: "non-increasing IDs", data: notIncreasing, postingCount: 2, totalDocuments: 2},
		{name: "wrong last document ID", data: wrongLastDocumentID, postingCount: 2, totalDocuments: 2},
		{name: "truncated payload", data: []byte(rawPostingBlockFixture[:len(rawPostingBlockFixture)-1]), postingCount: 2, totalDocuments: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readRawPostingBlock(bytes.NewReader(test.data), test.postingCount, test.totalDocuments); err == nil {
				t.Fatal("readRawPostingBlock() error = nil")
			}
		})
	}
}
