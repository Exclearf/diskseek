package segment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestWritePersistentDocumentFilesReadsSidecar(t *testing.T) {
	documents := persistentIndexTestDocuments()

	var gotLengths, gotOffsets, gotData bytes.Buffer
	gotMetadata, err := writePersistentDocumentFiles(
		context.Background(),
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
		context.Background(),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{documentCount: 2}, terms)),
		&gotTerms,
		&gotPostings,
		indexfile.PostingsCodecVByte,
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotMetadata.Codec != indexfile.PostingsCodecVByte {
		t.Fatalf("postings codec = %d, want %d", gotMetadata.Codec, indexfile.PostingsCodecVByte)
	}

	termIndex := 0
	postingIndex := 0
	var wantTerms, wantPostings bytes.Buffer
	wantMetadata, err := indexfile.WriteTermFiles(
		&wantTerms,
		&wantPostings,
		indexfile.PostingsCodecVByte,
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

func TestWritePersistentTermFilesStopsWhenCanceled(t *testing.T) {
	data := encodeHotTermRun(t, 0, 1<<13)
	ctx, cancel := context.WithCancel(context.Background())
	input := &cancelingReader{
		Reader:           bytes.NewReader(data),
		cancel:           cancel,
		readsUntilCancel: 2,
	}

	_, err := writePersistentTermFiles(
		ctx,
		input,
		io.Discard,
		io.Discard,
		indexfile.PostingsCodecRaw,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writePersistentTermFiles() error = %v, want %v", err, context.Canceled)
	}
	if input.readBytes > 2*runBufferBytes {
		t.Fatalf("bytes read after cancellation = %d, want at most %d", input.readBytes, 2*runBufferBytes)
	}
}

func TestWriteIndexCreatesGoldenCodecDirectories(t *testing.T) {
	runPath, documentsPath := writeIndexTestSources(t)
	tests := []struct {
		name    string
		fixture string
		codec   indexfile.PostingsCodec
	}{
		{"raw", "raw", indexfile.PostingsCodecRaw},
		{"vbyte", "vbyte", indexfile.PostingsCodecVByte},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "index")
			if err := writeIndex(context.Background(), destination, runPath, documentsPath, test.codec); err != nil {
				t.Fatal(err)
			}

			golden := filepath.Join("..", "indexfile", "testdata", "golden-v1", test.fixture)
			if !maps.EqualFunc(readIndexDirectory(t, destination), readIndexDirectory(t, golden), bytes.Equal) {
				t.Fatal("written index differs from golden index")
			}
		})
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

	if err := writeIndex(context.Background(), destination, runPath, documentsPath, indexfile.PostingsCodecVByte); err == nil {
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

	if err := writeIndex(context.Background(), destination, runPath, documentsPath, indexfile.PostingsCodecVByte); err == nil {
		t.Fatal("writeIndex() error = nil")
	}
	if data, err := os.ReadFile(markerPath); err != nil || string(data) != "keep" {
		t.Fatalf("existing destination changed: data %q, error %v", data, err)
	}
}

func TestPersistentIndexDoesNotDependOnSegmentLayout(t *testing.T) {
	input, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}

	oneRun, oneRunCount := buildPersistentTestIndex(t, input, math.MaxUint64)
	multipass, multipassRunCount := buildPersistentTestIndex(t, input, segmentBufferBytes)
	if oneRunCount != 1 {
		t.Fatalf("one-run build created %d runs", oneRunCount)
	}
	if multipassRunCount <= 2 {
		t.Fatalf("multipass build created %d runs, want more than fan-in 2", multipassRunCount)
	}
	if !maps.EqualFunc(readIndexDirectory(t, oneRun), readIndexDirectory(t, multipass), bytes.Equal) {
		t.Fatal("persistent index directories differ")
	}
	if err := indexfile.Verify(context.Background(), oneRun); err != nil {
		t.Fatalf("verify persistent index: %v", err)
	}
}

func buildPersistentTestIndex(t *testing.T, input []byte, flushTarget uint64) (string, int) {
	t.Helper()

	result, err := build(
		context.Background(),
		corpus.NewTSVReader(bytes.NewReader(input)),
		flushTarget,
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	runCount := len(result.runPaths)
	mergedRun, _, err := mergeRuns(context.Background(), result.directory, result.runPaths, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "index")
	if err := writeIndex(context.Background(), destination, mergedRun, result.documentsPath, indexfile.PostingsCodecVByte); err != nil {
		t.Fatal(err)
	}
	return destination, runCount
}

func readIndexDirectory(t *testing.T, directory string) map[string][]byte {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = data
	}
	return files
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
