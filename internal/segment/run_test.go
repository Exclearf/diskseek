package segment

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestWriteRunHeader(t *testing.T) {
	var output bytes.Buffer
	header := runHeader{firstDocumentID: 7, documentCount: 3}
	if err := writeRunHeader(&output, header); err != nil {
		t.Fatal(err)
	}

	const want = "44534b52554e3031070000000300000000000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("header bytes = %s, want %s", got, want)
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
