package segment

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestSegmentStateAddsDocuments(t *testing.T) {
	segment := newSegmentState(7)
	if got, want := segment.addDocument([]string{"a", "a"}), segmentBufferBytes+14; got != want {
		t.Fatalf("first document high point = %d, want %d", got, want)
	}
	if got, want := segment.addDocument([]string{"a", "b"}), segmentBufferBytes+36; got != want {
		t.Fatalf("second document high point = %d, want %d", got, want)
	}
	if got, want := segment.addDocument(nil), segmentBufferBytes+26; got != want {
		t.Fatalf("empty document high point = %d, want %d", got, want)
	}

	wantPostings := map[string][]index.Posting{
		"a": {
			{DocumentID: 7, Frequency: 2},
			{DocumentID: 8, Frequency: 1},
		},
		"b": {{DocumentID: 8, Frequency: 1}},
	}
	if !reflect.DeepEqual(segment.postings, wantPostings) {
		t.Fatalf("postings = %#v, want %#v", segment.postings, wantPostings)
	}
	if segment.firstDocumentID != 7 || segment.documentCount != 3 {
		t.Fatalf("document interval = [%d, %d), want [7, 10)", segment.firstDocumentID, uint64(segment.firstDocumentID)+segment.documentCount)
	}
}

func TestSegmentStateWritesRun(t *testing.T) {
	segment := newSegmentState(7)
	segment.addDocument([]string{"b", "a", "b"})
	segment.addDocument([]string{"c", "a"})

	output := &bufferWriteCloser{}
	if err := segment.writeRun(output); err != nil {
		t.Fatal(err)
	}

	reader, err := newRunReader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := runHeader{firstDocumentID: 7, documentCount: 2}
	if reader.header != wantHeader {
		t.Fatalf("header = %+v, want %+v", reader.header, wantHeader)
	}

	wantTerms := []struct {
		term     string
		postings []index.Posting
	}{
		{term: "a", postings: []index.Posting{{DocumentID: 7, Frequency: 1}, {DocumentID: 8, Frequency: 1}}},
		{term: "b", postings: []index.Posting{{DocumentID: 7, Frequency: 2}}},
		{term: "c", postings: []index.Posting{{DocumentID: 8, Frequency: 1}}},
	}
	for _, wantTerm := range wantTerms {
		term, postingCount, err := reader.nextTerm()
		if err != nil {
			t.Fatal(err)
		}
		if term != wantTerm.term || postingCount != uint64(len(wantTerm.postings)) {
			t.Fatalf("term header = (%q, %d), want (%q, %d)", term, postingCount, wantTerm.term, len(wantTerm.postings))
		}

		for _, wantPosting := range wantTerm.postings {
			posting, err := reader.nextPosting()
			if err != nil {
				t.Fatal(err)
			}
			if posting != wantPosting {
				t.Fatalf("posting = %+v, want %+v", posting, wantPosting)
			}
		}
	}
	if _, _, err := reader.nextTerm(); !errors.Is(err, io.EOF) {
		t.Fatalf("final nextTerm() error = %v, want EOF", err)
	}
}
