package indexfile

import (
	"errors"
	"math"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestCursorAdvanceMatchesLinearOracle(t *testing.T) {
	postings := spacedCursorTestPostings(postingsPerBlock*2 + 4)

	lastDocumentID := postings[len(postings)-1].DocumentID
	for target := index.DocumentID(0); target <= lastDocumentID+1; target++ {
		cursor := newRawCursorForTest(t, postings)
		valid, err := cursor.Advance(target)
		if err != nil {
			t.Fatal(err)
		}

		want, wantValid := linearAdvance(postings, target)
		if valid != wantValid {
			t.Fatalf("Advance(%d) valid = %t, want %t", target, valid, wantValid)
		}
		got, gotValid := cursor.Current()
		if gotValid != wantValid || got != want {
			t.Fatalf("Advance(%d) Current() = (%+v, %t), want (%+v, %t)", target, got, gotValid, want, wantValid)
		}
	}
}

func TestCursorAdvanceOperationSequence(t *testing.T) {
	postings := spacedCursorTestPostings(postingsPerBlock*2 + 4)
	cursor := newRawCursorForTest(t, postings)

	for _, step := range []struct {
		target index.DocumentID
		want   index.DocumentID
	}{
		{target: 7, want: 8},
		{target: 3, want: 8},
		{target: 255, want: 256},
		{target: 507, want: 508},
	} {
		valid, err := cursor.Advance(step.target)
		if err != nil || !valid {
			t.Fatalf("Advance(%d) = (%t, %v), want (true, nil)", step.target, valid, err)
		}
		posting, _ := cursor.Current()
		if posting.DocumentID != step.want {
			t.Fatalf("Advance(%d) document = %d, want %d", step.target, posting.DocumentID, step.want)
		}
	}

	if valid, err := cursor.Next(); err != nil || !valid {
		t.Fatalf("Next() = (%t, %v), want (true, nil)", valid, err)
	}
	if posting, _ := cursor.Current(); posting.DocumentID != 510 {
		t.Fatalf("Next() document = %d, want 510", posting.DocumentID)
	}

	if valid, err := cursor.Next(); err != nil || !valid {
		t.Fatalf("Next() = (%t, %v), want (true, nil)", valid, err)
	}
	if posting, _ := cursor.Current(); posting.DocumentID != 512 {
		t.Fatalf("Next() document = %d, want 512", posting.DocumentID)
	}
	if valid, err := cursor.Advance(517); err != nil || !valid {
		t.Fatalf("Advance(517) = (%t, %v), want (true, nil)", valid, err)
	}
	if posting, _ := cursor.Current(); posting.DocumentID != 518 {
		t.Fatalf("Advance(517) document = %d, want 518", posting.DocumentID)
	}

	if valid, err := cursor.Advance(math.MaxUint32); err != nil || valid {
		t.Fatalf("Advance(max uint32) = (%t, %v), want (false, nil)", valid, err)
	}
	if _, valid := cursor.Current(); valid {
		t.Fatal("Current() valid = true after exhaustion")
	}
	if valid, err := cursor.Advance(0); err != nil || valid {
		t.Fatalf("Advance after exhaustion = (%t, %v), want (false, nil)", valid, err)
	}
}

func TestCursorAdvanceSkipsIntermediatePayload(t *testing.T) {
	cursor := newRawCursorForTest(t, spacedCursorTestPostings(postingsPerBlock*2+4))

	valid, err := cursor.Advance(511)
	if err != nil || !valid {
		t.Fatalf("Advance(511) = (%t, %v), want (true, nil)", valid, err)
	}
	posting, _ := cursor.Current()
	if posting.DocumentID != 512 {
		t.Fatalf("Advance(511) document = %d, want 512", posting.DocumentID)
	}

	want := CursorStats{
		AdvanceCalls:     1,
		BlockHeadersRead: 3,
		BlocksSkipped:    1,
		BlocksDecoded:    2,
		PostingsDecoded:  postingsPerBlock + 4,
		BytesRequested:   3*postingBlockHeaderBytes + (postingsPerBlock+4)*rawPostingBytes,
	}
	if got := cursor.Stats(); got != want {
		t.Fatalf("Stats() = %+v, want %+v", got, want)
	}
}

func TestCursorAdvanceReadFailureInvalidatesCursor(t *testing.T) {
	for _, test := range []struct {
		name       string
		failedRead int
	}{
		{name: "header", failedRead: 1},
		{name: "payload", failedRead: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			cursor := newRawCursorForTest(t, cursorTestPostings(postingsPerBlock+1))
			input := cursor.input
			readErr := errors.New("read failed")
			reads := 0
			cursor.input = readerAtFunc(func(data []byte, offset int64) (int, error) {
				reads++
				if reads == test.failedRead {
					return 0, readErr
				}
				return input.ReadAt(data, offset)
			})

			valid, err := cursor.Advance(postingsPerBlock)
			if valid || !errors.Is(err, readErr) {
				t.Fatalf("Advance() = (%t, %v), want (false, %v)", valid, err, readErr)
			}
			if _, valid := cursor.Current(); valid {
				t.Fatal("Current() valid = true after read failure")
			}
		})
	}
}

func spacedCursorTestPostings(count int) []index.Posting {
	postings := make([]index.Posting, count)
	for position := range postings {
		postings[position] = index.Posting{
			DocumentID: index.DocumentID(position * 2),
			Frequency:  uint32(position%3 + 1),
		}
	}
	return postings
}

func linearAdvance(postings []index.Posting, target index.DocumentID) (index.Posting, bool) {
	for _, posting := range postings {
		if posting.DocumentID >= target {
			return posting, true
		}
	}
	return index.Posting{}, false
}
