package segment

import (
	"bytes"
	"io"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestWritePersistentDocumentFilesReadsSidecar(t *testing.T) {
	documents := []index.DocumentMeta{
		{ExternalID: "a", Length: 2},
		{ExternalID: "b", Length: 3},
	}

	var gotLengths, gotOffsets, gotData bytes.Buffer
	gotMetadata, err := writePersistentDocumentFiles(
		bytes.NewReader(encodeDocuments(t, documents)),
		&gotLengths,
		&gotOffsets,
		&gotData,
	)
	if err != nil {
		t.Fatal(err)
	}

	var wantLengths, wantOffsets, wantData bytes.Buffer
	next := 0
	wantMetadata, err := indexfile.WriteDocumentFiles(
		&wantLengths,
		&wantOffsets,
		&wantData,
		func() (index.DocumentMeta, error) {
			if next == len(documents) {
				return index.DocumentMeta{}, io.EOF
			}
			document := documents[next]
			next++
			return document, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(gotLengths.Bytes(), wantLengths.Bytes()) ||
		!bytes.Equal(gotOffsets.Bytes(), wantOffsets.Bytes()) ||
		!bytes.Equal(gotData.Bytes(), wantData.Bytes()) {
		t.Fatal("sidecar-driven document files do not match direct encoding")
	}
	if gotMetadata != wantMetadata {
		t.Fatalf("metadata = %+v, want %+v", gotMetadata, wantMetadata)
	}
}

func TestWritePersistentTermFilesReadsRun(t *testing.T) {
	terms := []mergeTestTerm{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
		},
		{
			term:     "search",
			postings: []index.Posting{{DocumentID: 0, Frequency: 1}},
		},
	}

	var gotTerms, gotPostings bytes.Buffer
	gotMetadata, err := writePersistentTermFiles(
		bytes.NewReader(encodeMergeTestRun(t, runHeader{documentCount: 2}, terms)),
		&gotTerms,
		&gotPostings,
	)
	if err != nil {
		t.Fatal(err)
	}

	termIndex := 0
	postingIndex := 0
	var wantTerms, wantPostings bytes.Buffer
	wantMetadata, err := indexfile.WriteTermFiles(
		&wantTerms,
		&wantPostings,
		func() (string, uint64, error) {
			if termIndex == len(terms) {
				return "", 0, io.EOF
			}
			term := terms[termIndex]
			termIndex++
			postingIndex = 0
			return term.term, uint64(len(term.postings)), nil
		},
		func() (index.Posting, error) {
			postings := terms[termIndex-1].postings
			if postingIndex == len(postings) {
				return index.Posting{}, io.EOF
			}
			posting := postings[postingIndex]
			postingIndex++
			return posting, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(gotTerms.Bytes(), wantTerms.Bytes()) ||
		!bytes.Equal(gotPostings.Bytes(), wantPostings.Bytes()) {
		t.Fatal("run-driven term files do not match direct encoding")
	}
	if gotMetadata != wantMetadata {
		t.Fatalf("metadata = %+v, want %+v", gotMetadata, wantMetadata)
	}
}
