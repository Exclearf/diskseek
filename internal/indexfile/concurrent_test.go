package indexfile

import (
	"slices"
	"sync"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestIndexSupportsConcurrentReaders(t *testing.T) {
	opened, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()

	tests := []struct {
		term        string
		postings    []index.Posting
		externalIDs []string
	}{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
			externalIDs: []string{"a", "b"},
		},
		{
			term:        "search",
			postings:    []index.Posting{{DocumentID: 0, Frequency: 1}},
			externalIDs: []string{"a"},
		},
	}

	start := make(chan struct{})
	var readers sync.WaitGroup
	for reader := range 16 {
		test := tests[reader%len(tests)]
		readers.Go(func() {
			<-start

			cursor, found, err := opened.Postings(test.term)
			if err != nil {
				t.Errorf("Postings(%q): %v", test.term, err)
				return
			}
			if !found {
				t.Errorf("Postings(%q) found = false", test.term)
				return
			}

			var postings []index.Posting
			var externalIDs []string
			for {
				posting, valid := cursor.Current()
				if !valid {
					break
				}
				postings = append(postings, posting)

				externalID, err := opened.ExternalID(posting.DocumentID)
				if err != nil {
					t.Errorf("ExternalID(%d): %v", posting.DocumentID, err)
					return
				}
				externalIDs = append(externalIDs, externalID)

				if _, err := cursor.Next(); err != nil {
					t.Errorf("Next(%q): %v", test.term, err)
					return
				}
			}

			if !slices.Equal(postings, test.postings) {
				t.Errorf("postings for %q = %+v, want %+v", test.term, postings, test.postings)
			}
			if !slices.Equal(externalIDs, test.externalIDs) {
				t.Errorf("external IDs for %q = %q, want %q", test.term, externalIDs, test.externalIDs)
			}
		})
	}
	close(start)
	readers.Wait()
}
