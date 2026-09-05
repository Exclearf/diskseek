package indexfile

import (
	"bytes"
	"testing"
)

func TestDocumentOffsetBytes(t *testing.T) {
	var encoded bytes.Buffer
	for _, offset := range []uint64{0, 1, 2} {
		if err := writeDocumentOffset(&encoded, offset); err != nil {
			t.Fatal(err)
		}
	}

	want := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("document offsets = % x, want % x", encoded.Bytes(), want)
	}

	for _, want := range []uint64{0, 1, 2} {
		got, err := readDocumentOffset(&encoded)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("document offset = %d, want %d", got, want)
		}
	}
}

func TestReadDocumentOffsetRejectsTruncatedValue(t *testing.T) {
	if _, err := readDocumentOffset(bytes.NewReader(make([]byte, 7))); err == nil {
		t.Fatal("readDocumentOffset() error = nil")
	}
}
