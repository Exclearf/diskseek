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

const goldenIndexDirectory = "testdata/golden-v1"

func readGoldenIndexFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(goldenIndexDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
