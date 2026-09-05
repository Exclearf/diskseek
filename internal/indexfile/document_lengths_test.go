package indexfile

import (
	"bytes"
	"testing"
)

func TestDocumentLengthBytes(t *testing.T) {
	var encoded bytes.Buffer
	for _, length := range []uint32{2, 3} {
		if err := writeDocumentLength(&encoded, length); err != nil {
			t.Fatal(err)
		}
	}

	want := []byte{
		0x02, 0x00, 0x00, 0x00,
		0x03, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("document lengths = % x, want % x", encoded.Bytes(), want)
	}

	for _, want := range []uint32{2, 3} {
		got, err := readDocumentLength(&encoded)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("document length = %d, want %d", got, want)
		}
	}
}

func TestReadDocumentLengthRejectsTruncatedValue(t *testing.T) {
	if _, err := readDocumentLength(bytes.NewReader([]byte{1, 0, 0})); err == nil {
		t.Fatal("readDocumentLength() error = nil")
	}
}
