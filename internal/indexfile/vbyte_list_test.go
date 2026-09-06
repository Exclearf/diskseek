package indexfile

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestVBytePostingListPartitionsBlocks(t *testing.T) {
	tests := []struct {
		postingCount int
		wantBytes    uint64
	}{
		{postingCount: 1, wantBytes: 10},
		{postingCount: 127, wantBytes: 262},
		{postingCount: 128, wantBytes: 264},
		{postingCount: 129, wantBytes: 275},
	}

	for _, test := range tests {
		t.Run(strconv.Itoa(test.postingCount), func(t *testing.T) {
			postings := make([]index.Posting, test.postingCount)
			for documentID := range postings {
				postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
			}

			next := 0
			var encoded bytes.Buffer
			var blockBuffer [postingBlockHeaderBytes + maxVBytePostingPayloadBytes]byte
			writtenBytes, err := writeVBytePostingList(&encoded, blockBuffer[:], uint64(len(postings)), func() (index.Posting, error) {
				if next == len(postings) {
					return index.Posting{}, io.EOF
				}
				posting := postings[next]
				next++
				return posting, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if writtenBytes != test.wantBytes || uint64(encoded.Len()) != test.wantBytes {
				t.Fatalf("encoded bytes = %d, returned = %d, want %d", encoded.Len(), writtenBytes, test.wantBytes)
			}

			var decoded []index.Posting
			if err := readVBytePostingList(
				bytes.NewReader(encoded.Bytes()),
				uint64(len(postings)),
				writtenBytes,
				uint64(len(postings)),
				func(posting index.Posting) error {
					decoded = append(decoded, posting)
					return nil
				},
			); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(decoded, postings) {
				t.Fatal("decoded postings do not match input")
			}
		})
	}
}

func TestReadVBytePostingListRejectsInvalidRangeAndOrder(t *testing.T) {
	postings := make([]index.Posting, postingsPerBlock+1)
	for documentID := range postings {
		postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
	}

	next := 0
	var encoded bytes.Buffer
	var blockBuffer [postingBlockHeaderBytes + maxVBytePostingPayloadBytes]byte
	writtenBytes, err := writeVBytePostingList(&encoded, blockBuffer[:], uint64(len(postings)), func() (index.Posting, error) {
		posting := postings[next]
		next++
		return posting, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	visit := func(index.Posting) error { return nil }
	if err := readVBytePostingList(
		bytes.NewReader(encoded.Bytes()),
		uint64(len(postings)),
		writtenBytes-1,
		uint64(len(postings)),
		visit,
	); err == nil {
		t.Fatal("readVBytePostingList() error = nil for short term range")
	}
	if err := readVBytePostingList(
		bytes.NewReader(append(encoded.Bytes(), 0)),
		uint64(len(postings)),
		writtenBytes+1,
		uint64(len(postings)),
		visit,
	); err == nil {
		t.Fatal("readVBytePostingList() error = nil for long term range")
	}

	encoded.Reset()
	if _, err := writeVBytePostingBlock(&encoded, blockBuffer[:], postings[:postingsPerBlock]); err != nil {
		t.Fatal(err)
	}
	if _, err := writeVBytePostingBlock(&encoded, blockBuffer[:], []index.Posting{{DocumentID: 0, Frequency: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := readVBytePostingList(
		bytes.NewReader(encoded.Bytes()),
		uint64(len(postings)),
		uint64(encoded.Len()),
		uint64(len(postings)),
		visit,
	); err == nil {
		t.Fatal("readVBytePostingList() error = nil for non-increasing block boundary")
	}
}

func TestWriteVBytePostingListRejectsInvalidCountAndShortSource(t *testing.T) {
	var blockBuffer [postingBlockHeaderBytes + maxVBytePostingPayloadBytes]byte
	if _, err := writeVBytePostingList(io.Discard, blockBuffer[:], 0, nil); err == nil {
		t.Fatal("writeVBytePostingList() error = nil for zero postings")
	}
	if _, err := writeVBytePostingList(io.Discard, blockBuffer[:], maxPostingsPerList+1, nil); err == nil {
		t.Fatal("writeVBytePostingList() error = nil for oversized list")
	}

	read := 0
	if _, err := writeVBytePostingList(io.Discard, blockBuffer[:], 2, func() (index.Posting, error) {
		if read == 1 {
			return index.Posting{}, io.EOF
		}
		read++
		return index.Posting{Frequency: 1}, nil
	}); err == nil {
		t.Fatal("writeVBytePostingList() error = nil for short source")
	}
}
