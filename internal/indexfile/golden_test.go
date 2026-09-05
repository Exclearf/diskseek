package indexfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyGoldenIndex(t *testing.T) {
	if err := verifyIndex(context.Background(), goldenIndexDirectory); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGoldenIndexRejectsTruncation(t *testing.T) {
	for _, name := range goldenIndexFileNames {
		t.Run(name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			for size := len(readGoldenIndexFile(t, name)) - 1; size >= 0; size-- {
				if err := os.Truncate(filepath.Join(directory, name), int64(size)); err != nil {
					t.Fatal(err)
				}
				if err := verifyIndex(context.Background(), directory); err == nil {
					t.Fatalf("verifyIndex() error = nil after truncating %s to %d bytes", name, size)
				}
			}
		})
	}
}

const goldenIndexDirectory = "testdata/golden-v1"

var goldenIndexFileNames = [...]string{
	MetadataFileName,
	TermsFileName,
	PostingsFileName,
	DocumentLengthsFileName,
	DocumentOffsetsFileName,
	DocumentDataFileName,
}

func readGoldenIndexFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(goldenIndexDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyGoldenIndex(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.CopyFS(directory, os.DirFS(goldenIndexDirectory)); err != nil {
		t.Fatal(err)
	}
	return directory
}
