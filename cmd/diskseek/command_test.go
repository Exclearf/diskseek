package main

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const testVersion = "9.8.7"

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if got := execute([]string{"version"}, &stdout, &stderr, testVersion); got != 0 {
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

	if got := execute([]string{"build"}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "build") {
		t.Fatalf("stderr = %q, want it to identify the unknown command", stderr.String())
	}
}

func TestQueryCommand(t *testing.T) {
	indexPath := filepath.Join("..", "..", "internal", "indexfile", "testdata", "golden-v1", "vbyte")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := execute([]string{"query", indexPath, "search go", "--limit", "1"}, &stdout, &stderr, testVersion); got != 0 {
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
	if got := execute([]string{"query", indexPath, "go", "--limit", "0"}, &stdout, &stderr, testVersion); got == 0 {
		t.Fatal("exit status = 0 for a zero query limit")
	}
}
