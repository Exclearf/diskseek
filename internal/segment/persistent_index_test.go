package segment

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestWritePersistentDocumentFilesReadsSidecar(t *testing.T) {
	documents := persistentIndexTestDocuments()

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
	terms := persistentIndexTestTerms()

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

func TestWriteIndexRemovesIncompleteDirectory(t *testing.T) {
	directory := t.TempDir()
	runPath := filepath.Join(directory, "run")
	if err := os.WriteFile(runPath, []byte("invalid run"), 0o600); err != nil {
		t.Fatal(err)
	}
	documentsPath := filepath.Join(directory, "documents")
	if err := os.WriteFile(documentsPath, encodeDocuments(t, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "index")

	if err := writeIndex(destination, runPath, documentsPath); err == nil {
		t.Fatal("writeIndex() error = nil")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index directory still exists: %v", err)
	}
}

func TestWriteIndexPreservesExistingDestination(t *testing.T) {
	runPath, documentsPath := writeIndexTestSources(t)
	destination := t.TempDir()
	markerPath := filepath.Join(destination, "keep")
	if err := os.WriteFile(markerPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeIndex(destination, runPath, documentsPath); err == nil {
		t.Fatal("writeIndex() error = nil")
	}
	if data, err := os.ReadFile(markerPath); err != nil || string(data) != "keep" {
		t.Fatalf("existing destination changed: data %q, error %v", data, err)
	}
}

func writeIndexTestSources(t *testing.T) (string, string) {
	t.Helper()

	directory := t.TempDir()
	terms := persistentIndexTestTerms()
	runPath := filepath.Join(directory, "run")
	if err := os.WriteFile(
		runPath,
		encodeMergeTestRun(t, runHeader{documentCount: 2}, terms),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	documentsPath := filepath.Join(directory, "documents")
	if err := os.WriteFile(documentsPath, encodeDocuments(t, persistentIndexTestDocuments()), 0o600); err != nil {
		t.Fatal(err)
	}
	return runPath, documentsPath
}

func persistentIndexTestTerms() []mergeTestTerm {
	return []mergeTestTerm{
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
}

func persistentIndexTestDocuments() []index.DocumentMeta {
	return []index.DocumentMeta{
		{ExternalID: "a", Length: 2},
		{ExternalID: "b", Length: 3},
	}
}

func TestWriteIndexCreatesCompleteDirectory(t *testing.T) {
	runPath, documentsPath := writeIndexTestSources(t)
	destination := filepath.Join(t.TempDir(), "index")

	if err := writeIndex(destination, runPath, documentsPath); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name string
		size int64
	}{
		{"docids.dat", 14},
		{"docids.off", 36},
		{"doclens.bin", 20},
		{"index.meta", 80},
		{"postings.bin", 52},
		{"terms.bin", 60},
	}
	if len(entries) != len(want) {
		t.Fatalf("file count = %d, want %d", len(entries), len(want))
	}
	for position, entry := range entries {
		if entry.Name() != want[position].name {
			t.Fatalf("file %d = %q, want %q", position, entry.Name(), want[position].name)
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != want[position].size {
			t.Fatalf("%s size = %d, want %d", entry.Name(), info.Size(), want[position].size)
		}
	}
}
