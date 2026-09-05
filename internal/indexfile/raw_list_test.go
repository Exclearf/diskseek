package indexfile

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestRawPostingListPartitionsBlocks(t *testing.T) {
	for _, postingCount := range []int{1, 127, 128, 129, 257} {
		t.Run(strconv.Itoa(postingCount), func(t *testing.T) {
			postings := make([]index.Posting, postingCount)
			for documentID := range postings {
				postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
			}

			next := 0
			var encoded bytes.Buffer
			writtenBytes, err := writeRawPostingList(&encoded, uint64(postingCount), func() (index.Posting, error) {
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

			blockCount := (postingCount + rawPostingsPerBlock - 1) / rawPostingsPerBlock
			wantBytes := uint64(postingCount*rawPostingBytes + blockCount*rawPostingBlockHeaderBytes)
			if writtenBytes != wantBytes || uint64(encoded.Len()) != wantBytes {
				t.Fatalf("encoded bytes = %d, returned = %d, want %d", encoded.Len(), writtenBytes, wantBytes)
			}
			if next != postingCount {
				t.Fatalf("postings read = %d, want %d", next, postingCount)
			}

			input := bytes.NewReader(encoded.Bytes())
			var decoded []index.Posting
			if err := readRawPostingList(
				input,
				uint64(postingCount),
				writtenBytes,
				uint64(postingCount),
				func(posting index.Posting) error {
					decoded = append(decoded, posting)
					return nil
				},
			); err != nil {
				t.Fatal(err)
			}
			if input.Len() != 0 {
				t.Fatalf("unread encoded bytes = %d", input.Len())
			}
			if !slices.Equal(decoded, postings) {
				t.Fatal("decoded postings do not match input")
			}
		})
	}
}

func TestReadRawPostingListRejectsInvalidData(t *testing.T) {
	visit := func(index.Posting) error { return nil }
	if err := readRawPostingList(
		bytes.NewBufferString(rawPostingBlockFixture),
		2,
		uint64(len(rawPostingBlockFixture)-1),
		2,
		visit,
	); err == nil {
		t.Fatal("readRawPostingList() error = nil for wrong byte length")
	}

	postings := make([]index.Posting, rawPostingsPerBlock+1)
	for position := range rawPostingsPerBlock {
		postings[position] = index.Posting{DocumentID: index.DocumentID(position + 1), Frequency: 1}
	}
	postings[rawPostingsPerBlock] = index.Posting{DocumentID: 0, Frequency: 1}

	var encoded bytes.Buffer
	if err := writeRawPostingBlock(&encoded, postings[:rawPostingsPerBlock]); err != nil {
		t.Fatal(err)
	}
	if err := writeRawPostingBlock(&encoded, postings[rawPostingsPerBlock:]); err != nil {
		t.Fatal(err)
	}
	if err := readRawPostingList(
		bytes.NewReader(encoded.Bytes()),
		uint64(len(postings)),
		uint64(encoded.Len()),
		uint64(len(postings)),
		visit,
	); err == nil {
		t.Fatal("readRawPostingList() error = nil for non-increasing block boundary")
	}
}

func TestWriteRawPostingListRejectsInvalidCountAndShortSource(t *testing.T) {
	if _, err := writeRawPostingList(io.Discard, 0, nil); err == nil {
		t.Fatal("writeRawPostingList() error = nil for zero postings")
	}
	if _, err := writeRawPostingList(io.Discard, maxPostingsPerList+1, nil); err == nil {
		t.Fatal("writeRawPostingList() error = nil for oversized list")
	}

	read := 0
	if _, err := writeRawPostingList(io.Discard, 2, func() (index.Posting, error) {
		if read == 1 {
			return index.Posting{}, io.EOF
		}
		read++
		return index.Posting{Frequency: 1}, nil
	}); err == nil {
		t.Fatal("writeRawPostingList() error = nil for short source")
	}
}
