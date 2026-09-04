package segment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
)

func TestBuildCreatesOwnedArtifacts(t *testing.T) {
	parent := t.TempDir()
	result, err := build(
		corpus.NewTSVReader(strings.NewReader("0\ta\n")),
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
	wantStats := buildStats{documentCount: 1, documentsWithTerms: 1, totalTokenCount: 1}
	if result.stats != wantStats {
		t.Fatalf("statistics = %+v, want %+v", result.stats, wantStats)
	}
}

func TestBuildRemovesOnlyOwnedArtifactsAfterCorpusError(t *testing.T) {
	parent := t.TempDir()
	keep := filepath.Join(parent, "keep")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := build(
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

func TestBuildRejectsZeroFlushTargetWithoutCreatingArtifacts(t *testing.T) {
	parent := t.TempDir()
	_, err := build(
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
