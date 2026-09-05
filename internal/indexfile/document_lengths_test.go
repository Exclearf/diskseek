package indexfile

import (
	"bytes"
	"slices"
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

func TestReadDocumentLengths(t *testing.T) {
	tests := []struct {
		name               string
		lengths            []uint32
		documentsWithTerms uint64
		totalLength        uint64
	}{
		{name: "empty"},
		{
			name:               "derived statistics",
			lengths:            []uint32{2, 0, 3},
			documentsWithTerms: 2,
			totalLength:        5,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := writeDocumentLengthsTestFile(t, test.lengths)
			got, err := readDocumentLengths(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.values, test.lengths) {
				t.Fatalf("lengths = %v, want %v", got.values, test.lengths)
			}
			if got.documentsWithTerms != test.documentsWithTerms {
				t.Fatalf(
					"documents with terms = %d, want %d",
					got.documentsWithTerms,
					test.documentsWithTerms,
				)
			}
			if got.totalLength != test.totalLength {
				t.Fatalf("total length = %d, want %d", got.totalLength, test.totalLength)
			}
		})
	}
}

func TestReadDocumentLengthsRejectsInvalidFile(t *testing.T) {
	t.Run("partial record", func(t *testing.T) {
		data := writeIndexFileTestData(t, documentLengthsRole, []byte{1})
		if _, err := readDocumentLengths(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("readDocumentLengths() error = nil")
		}
	})

	t.Run("body checksum", func(t *testing.T) {
		data := writeDocumentLengthsTestFile(t, []uint32{2})
		data[fileHeaderBytes] ^= 1
		if _, err := readDocumentLengths(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("readDocumentLengths() error = nil")
		}
	})

	t.Run("document count", func(t *testing.T) {
		size := int64(minimumFileBytes + (maxDocumentCount+1)*documentLengthBytes)
		if _, err := readDocumentLengths(bytes.NewBufferString("DSKDLEN\x01"), size); err == nil {
			t.Fatal("readDocumentLengths() error = nil")
		}
	})
}

func writeDocumentLengthsTestFile(t *testing.T, lengths []uint32) []byte {
	t.Helper()

	var body bytes.Buffer
	for _, length := range lengths {
		if err := writeDocumentLength(&body, length); err != nil {
			t.Fatal(err)
		}
	}
	return writeIndexFileTestData(t, documentLengthsRole, body.Bytes())
}

func writeIndexFileTestData(t *testing.T, role fileRole, body []byte) []byte {
	t.Helper()

	var output bytes.Buffer
	writer, err := newFileWriter(&output, role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(body); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.finish(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
