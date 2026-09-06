package segment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestBuildCreatesOwnedArtifacts(t *testing.T) {
	parent := t.TempDir()
	result, err := build(
		context.Background(),
		corpus.NewTSVReader(strings.NewReader("0\tx\n")),
		segmentBufferBytes,
		parent,
	)
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Dir(result.directory) != parent {
		t.Fatalf("build directory = %q, want child of %q", result.directory, parent)
	}
	if len(result.runPaths) != 1 {
		t.Fatalf("run count = %d, want 1", len(result.runPaths))
	}
	for _, path := range append([]string{result.documentsPath}, result.runPaths...) {
		if filepath.Dir(path) != result.directory {
			t.Fatalf("artifact path = %q, want child of %q", path, result.directory)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat artifact %q: %v", path, err)
		}
	}
	wantStats := buildStats{
		documentCount:      1,
		documentsWithTerms: 1,
		totalTokenCount:    1,
		postingCount:       1,
		maxAccountedBytes:  segmentBufferBytes + 14,
	}
	if result.stats != wantStats {
		t.Fatalf("statistics = %+v, want %+v", result.stats, wantStats)
	}
}

func TestBuildIndexReportsBuildAndRemovesTemporaryArtifacts(t *testing.T) {
	temporaryDirectory := t.TempDir()
	destination := filepath.Join(t.TempDir(), "index")
	report, err := BuildIndex(
		context.Background(),
		corpus.NewTSVReader(strings.NewReader("0\tgo go\n1\tgo search\n2\t\n")),
		destination,
		BuildOptions{
			FlushTarget:        1,
			MergeFanIn:         2,
			MergeWorkers:       1,
			Codec:              indexfile.PostingsCodecVByte,
			TemporaryDirectory: temporaryDirectory,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantReport := BuildReport{
		Documents:                3,
		DocumentsWithTerms:       2,
		Tokens:                   4,
		Postings:                 3,
		MaxAccountedSegmentBytes: segmentBufferBytes + 40,
		RunCount:                 3,
		MergePasses:              2,
		MergeInputBytes:          222,
		MergeOutputBytes:         160,
	}
	if report != wantReport {
		t.Fatalf("build report = %+v, want %+v", report, wantReport)
	}
	if err := indexfile.Verify(context.Background(), destination); err != nil {
		t.Fatalf("verify built index: %v", err)
	}

	entries, err := os.ReadDir(temporaryDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary entries = %v, want none", entries)
	}
}

func TestBuildIndexRejectsInvalidOptionsBeforeReadingCorpus(t *testing.T) {
	valid := BuildOptions{
		FlushTarget:  1,
		MergeFanIn:   2,
		MergeWorkers: 1,
		Codec:        indexfile.PostingsCodecVByte,
	}
	tests := []struct {
		name       string
		invalidate func(*BuildOptions)
	}{
		{"flush target", func(options *BuildOptions) { options.FlushTarget = 0 }},
		{"fan-in", func(options *BuildOptions) { options.MergeFanIn = 1 }},
		{"workers", func(options *BuildOptions) { options.MergeWorkers = 0 }},
		{"codec", func(options *BuildOptions) { options.Codec = 3 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			input := strings.NewReader("unread")
			options := valid
			test.invalidate(&options)
			options.TemporaryDirectory = parent

			_, err := BuildIndex(
				context.Background(),
				corpus.NewTSVReader(input),
				filepath.Join(parent, "index"),
				options,
			)
			if err == nil {
				t.Fatal("BuildIndex() error = nil")
			}
			if input.Len() != len("unread") {
				t.Fatal("BuildIndex() read the corpus before rejecting its options")
			}

			entries, err := os.ReadDir(parent)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("created entries = %v, want none", entries)
			}
		})
	}
}

func TestBuildRemovesOnlyOwnedArtifactsAfterCorpusError(t *testing.T) {
	parent := t.TempDir()
	keep := filepath.Join(parent, "keep")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := build(
		context.Background(),
		corpus.NewTSVReader(strings.NewReader("0\ta\n1\tb\nbroken\n")),
		segmentBufferBytes,
		parent,
	)
	if !errors.Is(err, corpus.ErrMalformedRecord) {
		t.Fatalf("build() error = %v, want %v", err, corpus.ErrMalformedRecord)
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(keep) {
		t.Fatalf("temporary parent entries = %v, want only %q", entries, filepath.Base(keep))
	}
}

func TestBuildRemovesArtifactsAfterCancellation(t *testing.T) {
	parent := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	_, err := build(
		ctx,
		corpus.NewTSVReader(&cancelOnSecondRead{cancel: cancel}),
		segmentBufferBytes,
		parent,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("build() error = %v, want %v", err, context.Canceled)
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary parent entries = %v, want none", entries)
	}
}

func TestBuildRejectsZeroFlushTargetWithoutCreatingArtifacts(t *testing.T) {
	parent := t.TempDir()
	_, err := build(
		context.Background(),
		corpus.NewTSVReader(strings.NewReader("")),
		0,
		parent,
	)
	if err == nil {
		t.Fatal("build() error = nil")
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary parent entries = %v, want none", entries)
	}
}

func TestBuildLogicalOutputDoesNotDependOnFlushTarget(t *testing.T) {
	input, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	want, err := index.Build(corpus.NewTSVReader(bytes.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		target   uint64
		wantRuns int
	}{
		{name: "one run", target: math.MaxUint64, wantRuns: 1},
		{name: "one run per document", target: segmentBufferBytes, wantRuns: len(want.Documents)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := build(
				context.Background(),
				corpus.NewTSVReader(bytes.NewReader(input)),
				test.target,
				t.TempDir(),
			)
			if err != nil {
				t.Fatal(err)
			}

			got := readBuildIndex(t, result)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("logical index = %#v, want %#v", got, want)
			}
			if len(result.runPaths) != test.wantRuns {
				t.Fatalf("run count = %d, want %d", len(result.runPaths), test.wantRuns)
			}
			if result.stats.documentCount != uint64(len(want.Documents)) {
				t.Fatalf("document count = %d, want %d", result.stats.documentCount, len(want.Documents))
			}
		})
	}
}

func TestBuildArtifactBytesAreDeterministic(t *testing.T) {
	input, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}

	buildOnce := func() buildResult {
		result, err := build(
			context.Background(),
			corpus.NewTSVReader(bytes.NewReader(input)),
			math.MaxUint64,
			t.TempDir(),
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := buildOnce()
	second := buildOnce()
	if first.stats != second.stats {
		t.Fatalf("build statistics differ: %+v and %+v", first.stats, second.stats)
	}
	if !reflect.DeepEqual(readBuildArtifacts(t, first), readBuildArtifacts(t, second)) {
		t.Fatal("build artifact bytes differ")
	}
}

func readBuildArtifacts(t *testing.T, result buildResult) [][]byte {
	t.Helper()

	paths := append([]string{result.documentsPath}, result.runPaths...)
	artifacts := make([][]byte, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		artifacts[i] = data
	}
	return artifacts
}

func readBuildIndex(t *testing.T, result buildResult) index.Index {
	t.Helper()

	documentData, err := os.ReadFile(result.documentsPath)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := decodeDocuments(documentData)
	if err != nil {
		t.Fatal(err)
	}

	postings := make(map[string][]index.Posting)
	for _, path := range result.runPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := newRunReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for {
			term, postingCount, err := reader.nextTerm()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			for range postingCount {
				posting, err := reader.nextPosting()
				if err != nil {
					t.Fatal(err)
				}
				postings[term] = append(postings[term], posting)
			}
		}
	}

	return index.Index{
		Documents:          documents,
		Postings:           postings,
		DocumentsWithTerms: result.stats.documentsWithTerms,
		TotalLength:        result.stats.totalTokenCount,
	}
}

type cancelOnSecondRead struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelOnSecondRead) Read(buffer []byte) (int, error) {
	switch r.reads {
	case 0:
		r.reads++
		return copy(buffer, "0\ta\n"), nil
	case 1:
		r.reads++
		r.cancel()
		return copy(buffer, "1\tb\n"), nil
	default:
		return 0, io.EOF
	}
}
