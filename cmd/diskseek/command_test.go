package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

const testVersion = "9.8.7"

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if got := execute(t.Context(), []string{"version"}, &stdout, &stderr, testVersion); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if got, want := stdout.String(), "version="+testVersion+"\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no output", stderr.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if got := execute(t.Context(), []string{"build"}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "build") {
		t.Fatalf("stderr = %q, want it to identify the unknown command", stderr.String())
	}
}

func TestIndexCommand(t *testing.T) {
	directory := t.TempDir()
	corpusPath := filepath.Join(directory, "corpus.tsv")
	if err := os.WriteFile(corpusPath, []byte("a\tgo search\nb\tyak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(directory, "index")
	temporaryDirectory := filepath.Join(directory, "temporary")
	if err := os.Mkdir(temporaryDirectory, 0o700); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := execute(
		t.Context(),
		[]string{"index", corpusPath, indexPath, "--temp-dir", temporaryDirectory},
		&stdout,
		&stderr,
		testVersion,
	); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if err := indexfile.Verify(t.Context(), indexPath); err != nil {
		t.Fatalf("verify index: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if got := execute(t.Context(), []string{"query", indexPath, "go"}, &stdout, &stderr, testVersion); got != 0 {
		t.Fatalf("query exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "a\t") {
		t.Fatalf("query stdout = %q, want document a", stdout.String())
	}
}

func TestQueryCommand(t *testing.T) {
	indexPath := filepath.Join("..", "..", "internal", "indexfile", "testdata", "golden-v1", "vbyte")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := execute(t.Context(), []string{"query", indexPath, "search go", "--limit", "1"}, &stdout, &stderr, testVersion); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}

	fields := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\t")
	if len(fields) != 2 || fields[0] != "a" {
		t.Fatalf("stdout = %q, want document a and its score", stdout.String())
	}
	if _, err := strconv.ParseFloat(fields[1], 64); err != nil {
		t.Fatalf("score %q is not a float", fields[1])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no output", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if got := execute(t.Context(), []string{"query", indexPath, "go", "--limit", "0"}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0 for a zero query limit")
	}
}

func TestVerifyCommand(t *testing.T) {
	indexPath := filepath.Join("..", "..", "internal", "indexfile", "testdata", "golden-v1", "vbyte")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := execute(t.Context(), []string{"verify", indexPath}, &stdout, &stderr, testVersion); got != 0 {
		t.Fatalf("exit status = %d, want 0; stderr = %q", got, stderr.String())
	}
	if got, want := stdout.String(), "verified\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no output", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if got := execute(t.Context(), []string{"verify", filepath.Join(t.TempDir(), "missing")}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0 for a missing index")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stdout.Reset()
	stderr.Reset()
	if got := execute(ctx, []string{"verify", indexPath}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0 for a canceled verification")
	}
}
