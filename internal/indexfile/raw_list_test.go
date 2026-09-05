package indexfile

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestWriteRawPostingListPartitionsBlocks(t *testing.T) {
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

			var decoded []index.Posting
			remaining := postingCount
			for remaining != 0 {
				currentCount := min(remaining, rawPostingsPerBlock)
				block, err := readRawPostingBlock(&encoded, currentCount, uint64(postingCount))
				if err != nil {
					t.Fatal(err)
				}
				decoded = append(decoded, block...)
				remaining -= currentCount
			}
			if encoded.Len() != 0 {
				t.Fatalf("unread encoded bytes = %d", encoded.Len())
			}
			if !slices.Equal(decoded, postings) {
				t.Fatal("decoded postings do not match input")
			}
		})
	}
}

func TestWriteRawPostingListRejectsInvalidCountAndShortSource(t *testing.T) {
	if _, err := writeRawPostingList(io.Discard, 0, nil); err == nil {
		t.Fatal("writeRawPostingList() error = nil for zero postings")
	}
	if _, err := writeRawPostingList(io.Discard, maximumPostingsPerList+1, nil); err == nil {
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
