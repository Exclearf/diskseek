package main

import (
	"bytes"
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
