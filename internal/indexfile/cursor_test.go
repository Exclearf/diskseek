package indexfile

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestIndexPostings(t *testing.T) {
	opened, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()

	t.Run("known term", func(t *testing.T) {
		cursor, found, err := opened.Postings("go")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("Postings() found = false")
		}
		posting, valid := cursor.Current()
		if !valid || posting != (index.Posting{DocumentID: 0, Frequency: 1}) {
			t.Fatalf("Current() = (%+v, %t), want ({0 1}, true)", posting, valid)
		}
	})

	t.Run("unknown term", func(t *testing.T) {
		cursor, found, err := opened.Postings("missing")
		if err != nil {
			t.Fatal(err)
		}
		if found || cursor != nil {
			t.Fatalf("Postings() = (%v, %t), want (nil, false)", cursor, found)
		}
	})

	t.Run("term range not fully consumed", func(t *testing.T) {
		entry := opened.terms["go"]
		entry.postingsBytes++
		opened.terms["go"] = entry
		defer func() {
			entry.postingsBytes--
			opened.terms["go"] = entry
		}()

		cursor, found, err := opened.Postings("go")
		if err == nil {
			t.Fatal("Postings() error = nil")
		}
		if !found || cursor != nil {
			t.Fatalf("Postings() = (%v, %t), want (nil, true)", cursor, found)
		}
	})
}

func TestCursorNext(t *testing.T) {
	for _, postingCount := range []int{1, 127, 128, 129, 257} {
		t.Run(strconv.Itoa(postingCount), func(t *testing.T) {
			postings := cursorTestPostings(postingCount)
			cursor := newRawCursorForTest(t, postings)

			for position, want := range postings {
				if got := currentCursorPosting(t, cursor); got != want {
					t.Fatalf("posting %d = %+v, want %+v", position, got, want)
				}

				valid, err := cursor.Next()
				if err != nil {
					t.Fatal(err)
				}
				if wantValid := position+1 < len(postings); valid != wantValid {
					t.Fatalf("Next() valid = %t, want %t", valid, wantValid)
				}
			}

			if posting, valid := cursor.Current(); valid {
				t.Fatalf("Current() = (%+v, true) after exhaustion", posting)
			}
			for range 2 {
				if valid, err := cursor.Next(); valid || err != nil {
					t.Fatalf("Next() = (%t, %v), want (false, nil)", valid, err)
				}
			}
		})
	}
}

func TestCursorNextFailureInvalidatesCursor(t *testing.T) {
	cursor := newRawCursorForTest(t, cursorTestPostings(rawPostingsPerBlock+1))
	readErr := errors.New("read failed")
	cursor.input = readerAtFunc(func([]byte, int64) (int, error) {
		return 0, readErr
	})

	for range rawPostingsPerBlock - 1 {
		valid, err := cursor.Next()
		if err != nil || !valid {
			t.Fatalf("Next() = (%t, %v), want (true, nil)", valid, err)
		}
	}
	if valid, err := cursor.Next(); valid || !errors.Is(err, readErr) {
		t.Fatalf("Next() = (%t, %v), want (false, %v)", valid, err, readErr)
	}
	if posting, valid := cursor.Current(); valid {
		t.Fatalf("Current() = (%+v, true) after read failure", posting)
	}
}

func TestCursorNextRejectsNonIncreasingBlocks(t *testing.T) {
	postings := cursorTestPostings(rawPostingsPerBlock + 1)
	postings[rawPostingsPerBlock].DocumentID = 0
	cursor := newRawCursorForTest(t, postings)

	for range rawPostingsPerBlock - 1 {
		if _, err := cursor.Next(); err != nil {
			t.Fatal(err)
		}
	}
	valid, err := cursor.Next()
	if valid || err == nil {
		t.Fatalf("Next() = (%t, %v), want (false, error)", valid, err)
	}
	if posting, valid := cursor.Current(); valid {
		t.Fatalf("Current() = (%+v, true) after ordering failure", posting)
	}
}

func cursorTestPostings(count int) []index.Posting {
	postings := make([]index.Posting, count)
	for position := range postings {
		postings[position] = index.Posting{
			DocumentID: index.DocumentID(position),
			Frequency:  uint32(position%3 + 1),
		}
	}
	return postings
}

func currentCursorPosting(t *testing.T, cursor *Cursor) index.Posting {
	t.Helper()
	posting, valid := cursor.Current()
	if !valid {
		t.Fatal("Current() valid = false")
	}
	return posting
}

func newRawCursorForTest(t *testing.T, postings []index.Posting) *Cursor {
	t.Helper()

	var encoded bytes.Buffer
	next := 0
	postingsBytes, err := writeRawPostingList(&encoded, uint64(len(postings)), func() (index.Posting, error) {
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

	var documentCount int
	for _, posting := range postings {
		documentCount = max(documentCount, int(posting.DocumentID)+1)
	}
	documentLengths := make([]uint32, documentCount)
	for _, posting := range postings {
		documentLengths[posting.DocumentID] = max(documentLengths[posting.DocumentID], posting.Frequency)
	}
	data := append(make([]byte, fileHeaderBytes), encoded.Bytes()...)
	cursor := &Cursor{
		input:             bytes.NewReader(data),
		term:              termEntry{documentFrequency: uint64(len(postings)), postingsOffset: fileHeaderBytes, postingsBytes: postingsBytes},
		documentLengths:   documentLengths,
		nextBlockOffset:   fileHeaderBytes,
		postingsRemaining: uint64(len(postings)),
	}
	if err := cursor.loadBlock(); err != nil {
		t.Fatal(err)
	}
	return cursor
}
