package segment

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestRunHeaderRoundTripAndBytes(t *testing.T) {
	var output bytes.Buffer
	header := runHeader{documentCount: documentIDLimit}
	if err := writeRunHeader(&output, header); err != nil {
		t.Fatal(err)
	}

	const want = "44534b52554e3031000000000000000001000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("header bytes = %s, want %s", got, want)
	}

	got, err := readRunHeader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got != header {
		t.Fatalf("readRunHeader() = %+v, want %+v", got, header)
	}
}

func TestReadRunHeaderRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "wrong magic", data: make([]byte, runHeaderBytes)},
		{name: "truncated header", data: []byte(runMagic)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readRunHeader(bytes.NewReader(test.data)); err == nil {
				t.Fatal("readRunHeader() error = nil")
			}
		})
	}
}

func TestWriteRunTermHeader(t *testing.T) {
	var output bytes.Buffer
	if err := writeRunTermHeader(&output, runHeader{documentCount: documentIDLimit}, "go", documentIDLimit); err != nil {
		t.Fatal(err)
	}

	const want = "02000000676f0000000001000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("term bytes = %s, want %s", got, want)
	}
}

func TestWriteRunTermHeaderRejectsInvalidValues(t *testing.T) {
	header := runHeader{documentCount: 1}
	tests := []struct {
		name         string
		term         string
		postingCount uint64
	}{
		{name: "empty term", postingCount: 1},
		{name: "invalid UTF-8", term: string([]byte{0xff}), postingCount: 1},
		{name: "oversized term", term: strings.Repeat("a", maxRunTermBytes+1), postingCount: 1},
		{name: "zero posting count", term: "a"},
		{name: "posting count above document count", term: "a", postingCount: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := writeRunTermHeader(io.Discard, header, test.term, test.postingCount); err == nil {
				t.Fatal("writeRunTermHeader() error = nil")
			}
		})
	}
}

func TestRunHeaderDocumentInterval(t *testing.T) {
	header := runHeader{firstDocumentID: 7, documentCount: 3}
	if err := validateRunHeader(header); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		documentID index.DocumentID
		want       bool
	}{
		{documentID: 6},
		{documentID: 7, want: true},
		{documentID: 9, want: true},
		{documentID: 10},
	}
	for _, test := range tests {
		if got := documentInRun(header, test.documentID); got != test.want {
			t.Errorf("documentInRun(%d) = %t, want %t", test.documentID, got, test.want)
		}
	}
}

func TestRunHeaderRejectsInvalidInterval(t *testing.T) {
	tests := []runHeader{
		{firstDocumentID: 1},
		{firstDocumentID: index.DocumentID(documentIDLimit - 1), documentCount: 2},
	}

	for _, header := range tests {
		if err := validateRunHeader(header); err == nil {
			t.Errorf("validateRunHeader(%+v) error = nil", header)
		}
	}
}
