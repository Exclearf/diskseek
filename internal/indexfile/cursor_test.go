package indexfile

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/rand/v2"
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
			if got, want := cursor.Stats().NextCalls, uint64(len(postings)+2); got != want {
				t.Fatalf("NextCalls = %d, want %d", got, want)
			}
		})
	}
}

func TestCursorNextFailureInvalidatesCursor(t *testing.T) {
	cursor := newRawCursorForTest(t, cursorTestPostings(postingsPerBlock+1))
	readErr := errors.New("read failed")
	cursor.input = readerAtFunc(func([]byte, int64) (int, error) {
		return 0, readErr
	})

	for range postingsPerBlock - 1 {
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
	postings := cursorTestPostings(postingsPerBlock + 1)
	postings[postingsPerBlock].DocumentID = 0
	cursor := newRawCursorForTest(t, postings)

	for range postingsPerBlock - 1 {
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

func TestCursorCodecsMatchLogicalTraces(t *testing.T) {
	counts := []int{1, 127, 128, 129, 257}
	countGenerator := rand.New(rand.NewPCG(1, 1))
	for range 27 {
		counts = append(counts, countGenerator.IntN(512)+1)
	}

	for caseIndex, count := range counts {
		name := strconv.Itoa(caseIndex) + "-" + strconv.Itoa(count)
		t.Run(name, func(t *testing.T) {
			generator := rand.New(rand.NewPCG(2, uint64(caseIndex+1)))
			postings := generatedCursorPostings(generator, count)
			operations := generatedCursorOperations(generator, postings)

			cursors := []struct {
				name   string
				cursor *Cursor
			}{
				{"raw", newCursorForTest(t, postings, PostingsCodecRaw)},
				{"vbyte", newCursorForTest(t, postings, PostingsCodecVByte)},
			}

			position := 0
			for step, operation := range operations {
				if operation.advance {
					for position < len(postings) && postings[position].DocumentID < operation.target {
						position++
					}
				} else if position < len(postings) {
					position++
				}

				wantValid := position < len(postings)
				var want index.Posting
				if wantValid {
					want = postings[position]
				}
				for _, candidate := range cursors {
					var valid bool
					var err error
					if operation.advance {
						valid, err = candidate.cursor.Advance(operation.target)
					} else {
						valid, err = candidate.cursor.Next()
					}
					if err != nil {
						t.Fatalf("%s step %d operation %+v: %v", candidate.name, step, operation, err)
					}
					got, current := candidate.cursor.Current()
					if valid != wantValid || current != wantValid || got != want {
						t.Fatalf(
							"%s step %d operation %+v = (%+v, %t, %t), want (%+v, %t, %t)",
							candidate.name,
							step,
							operation,
							got,
							current,
							valid,
							want,
							wantValid,
							wantValid,
						)
					}
				}
			}
		})
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
	return newCursorForTest(t, postings, PostingsCodecRaw)
}

func newCursorForTest(t *testing.T, postings []index.Posting, codec PostingsCodec) *Cursor {
	t.Helper()
	return newCursorTestFixture(t, postings, codec).open(t)
}

type cursorTestFixture struct {
	data            []byte
	term            termEntry
	codec           PostingsCodec
	documentLengths []uint32
}

func newCursorTestFixture(t testing.TB, postings []index.Posting, codec PostingsCodec) cursorTestFixture {
	t.Helper()

	var encoded bytes.Buffer
	next := 0
	writePostingList := writeRawPostingList
	if codec == PostingsCodecVByte {
		writePostingList = writeVBytePostingList
	}
	postingsBytes, err := writePostingList(&encoded, uint64(len(postings)), func() (index.Posting, error) {
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
	return cursorTestFixture{
		data: append(make([]byte, fileHeaderBytes), encoded.Bytes()...),
		term: termEntry{
			documentFrequency: uint64(len(postings)),
			postingsOffset:    fileHeaderBytes,
			postingsBytes:     postingsBytes,
		},
		codec:           codec,
		documentLengths: documentLengths,
	}
}

func (f cursorTestFixture) open(t testing.TB) *Cursor {
	t.Helper()
	cursor := &Cursor{
		input:             bytes.NewReader(f.data),
		term:              f.term,
		codec:             f.codec,
		documentLengths:   f.documentLengths,
		nextBlockOffset:   fileHeaderBytes,
		postingsRemaining: f.term.documentFrequency,
	}
	if err := cursor.loadBlock(); err != nil {
		t.Fatal(err)
	}
	return cursor
}

type cursorOperation struct {
	advance bool
	target  index.DocumentID
}

func generatedCursorPostings(generator *rand.Rand, count int) []index.Posting {
	postings := make([]index.Posting, count)
	documentID := index.DocumentID(generator.Uint32N(300) + 1)
	for position := range postings {
		if position != 0 {
			documentID += index.DocumentID(generator.Uint32N(300) + 1)
		}
		postings[position] = index.Posting{
			DocumentID: documentID,
			Frequency:  generator.Uint32N(300) + 1,
		}
	}
	return postings
}

func generatedCursorOperations(generator *rand.Rand, postings []index.Posting) []cursorOperation {
	operations := []cursorOperation{
		{advance: true, target: postings[0].DocumentID},
		{advance: true, target: 0},
	}
	if len(postings) > 1 {
		operations = append(operations, cursorOperation{
			advance: true,
			target:  postings[1].DocumentID - 1,
		})
	}
	if len(postings) > postingsPerBlock {
		operations = append(
			operations,
			cursorOperation{advance: true, target: postings[postingsPerBlock-1].DocumentID},
			cursorOperation{},
		)
	}

	for range 64 {
		if generator.IntN(3) == 0 {
			operations = append(operations, cursorOperation{})
			continue
		}
		target := postings[generator.IntN(len(postings))].DocumentID
		switch generator.IntN(3) {
		case 0:
			target--
		case 1:
			target++
		}
		operations = append(operations, cursorOperation{advance: true, target: target})
	}
	return append(
		operations,
		cursorOperation{advance: true, target: math.MaxUint32},
		cursorOperation{},
		cursorOperation{advance: true, target: 0},
	)
}
