package segment

import (
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
