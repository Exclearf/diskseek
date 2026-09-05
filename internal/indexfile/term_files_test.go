package indexfile

import (
	"bytes"
	"io"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestWriteTermBodiesRecordsWrittenPostingLengths(t *testing.T) {
	input := []struct {
		term     string
		postings []index.Posting
	}{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
		},
		{
			term:     "yak",
			postings: []index.Posting{{DocumentID: 0, Frequency: 1}},
		},
	}

	termIndex := 0
	postingIndex := 0
	nextTerm := func() (string, uint64, error) {
		if termIndex == len(input) {
			return "", 0, io.EOF
		}
		current := input[termIndex]
		termIndex++
		postingIndex = 0
		return current.term, uint64(len(current.postings)), nil
	}
	nextPosting := func() (index.Posting, error) {
		current := input[termIndex-1].postings
		if postingIndex == len(current) {
			return index.Posting{}, io.EOF
		}
		posting := current[postingIndex]
		postingIndex++
		return posting, nil
	}

	var termBody, postingBody bytes.Buffer
	if err := writeTermBodies(&termBody, &postingBody, nextTerm, nextPosting); err != nil {
		t.Fatal(err)
	}

	wantTerms := append(append([]byte(nil), goTermRecord...), yakTermRecord...)
	if !bytes.Equal(termBody.Bytes(), wantTerms) {
		t.Fatalf("term body = % x, want % x", termBody.Bytes(), wantTerms)
	}
	if postingBody.Len() != 40 {
		t.Fatalf("posting body length = %d, want 40", postingBody.Len())
	}
}
