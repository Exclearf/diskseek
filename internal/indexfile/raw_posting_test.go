package indexfile

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestWriteRawPostingBlockBytes(t *testing.T) {
	postings := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 1, Frequency: 3},
	}
	want := []byte{
		1, 0, 0, 0,
		16, 0, 0, 0,
		0, 0, 0, 0, 1, 0, 0, 0,
		1, 0, 0, 0, 3, 0, 0, 0,
	}

	var encoded bytes.Buffer
	if err := writeRawPostingBlock(&encoded, postings); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("block = % x, want % x", encoded.Bytes(), want)
	}
}

func TestWriteRawPostingBlockCount(t *testing.T) {
	postings := make([]index.Posting, rawPostingsPerBlock)
	for documentID := range postings {
		postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
	}

	var encoded bytes.Buffer
	if err := writeRawPostingBlock(&encoded, postings); err != nil {
		t.Fatal(err)
	}
	if got, want := encoded.Len(), rawPostingBlockHeaderBytes+rawPostingsPerBlock*rawPostingBytes; got != want {
		t.Fatalf("block length = %d, want %d", got, want)
	}
	if got := binary.LittleEndian.Uint32(encoded.Bytes()[4:8]); got != rawPostingsPerBlock*rawPostingBytes {
		t.Fatalf("payload length = %d, want %d", got, rawPostingsPerBlock*rawPostingBytes)
	}

	if err := writeRawPostingBlock(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("writeRawPostingBlock() error = nil for empty block")
	}
	if err := writeRawPostingBlock(&bytes.Buffer{}, make([]index.Posting, rawPostingsPerBlock+1)); err == nil {
		t.Fatal("writeRawPostingBlock() error = nil for oversized block")
	}
}
