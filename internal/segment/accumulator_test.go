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
	accountedBytes, postingCount := segment.addDocument([]string{"a", "a"})
	if want := segmentBufferBytes + 14; accountedBytes != want || postingCount != 1 {
		t.Fatalf("first document statistics = (%d, %d), want (%d, 1)", accountedBytes, postingCount, want)
	}
	accountedBytes, postingCount = segment.addDocument([]string{"a", "b"})
	if want := segmentBufferBytes + 36; accountedBytes != want || postingCount != 2 {
		t.Fatalf("second document statistics = (%d, %d), want (%d, 2)", accountedBytes, postingCount, want)
	}
	accountedBytes, postingCount = segment.addDocument(nil)
	if want := segmentBufferBytes + 26; accountedBytes != want || postingCount != 0 {
		t.Fatalf("empty document statistics = (%d, %d), want (%d, 0)", accountedBytes, postingCount, want)
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
