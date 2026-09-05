package indexfile

import (
	"bytes"
	"testing"
)

func TestWriteTermRecordBytes(t *testing.T) {
	var encoded bytes.Buffer
	if err := writeTermRecord(&encoded, "go", 2, 24); err != nil {
		t.Fatal(err)
	}

	want := []byte{
		0x02, 0x00, 0x00, 0x00,
		'g', 'o',
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(encoded.Bytes(), want) {
		t.Fatalf("term record = % x, want % x", encoded.Bytes(), want)
	}
}
