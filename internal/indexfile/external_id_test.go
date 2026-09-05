package indexfile

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
)

func TestReadExternalID(t *testing.T) {
	got, err := readExternalID(strings.NewReader("source"), uint64(len("source")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "source" {
		t.Fatalf("external ID = %q, want %q", got, "source")
	}
}

func TestReadExternalIDRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		length uint64
	}{
		{name: "empty"},
		{name: "oversized", length: corpus.MaxExternalIDBytes + 1},
		{name: "invalid UTF-8", data: []byte{0xff}, length: 1},
		{name: "truncated", data: []byte("a"), length: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readExternalID(bytes.NewReader(test.data), test.length); err == nil {
				t.Fatal("readExternalID() error = nil")
			}
		})
	}
}
