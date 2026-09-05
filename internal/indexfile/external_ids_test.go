package indexfile

import (
	"bytes"
	"errors"
	"slices"
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

func TestVerifyExternalIDFiles(t *testing.T) {
	tests := []struct {
		name          string
		offsets       []uint64
		data          string
		documentCount uint64
	}{
		{name: "empty", offsets: []uint64{0}},
		{name: "documents", offsets: []uint64{0, 1, 3}, data: "abc", documentCount: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			offsets, data := writeExternalIDTestFiles(t, test.offsets, test.data)
			if err := verifyExternalIDFiles(
				bytes.NewReader(offsets), int64(len(offsets)),
				bytes.NewReader(data), int64(len(data)),
				test.documentCount,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerifyExternalIDFilesRejectsMismatch(t *testing.T) {
	t.Run("document count", func(t *testing.T) {
		offsets, data := writeExternalIDTestFiles(t, []uint64{0, 1, 3}, "abc")
		if err := verifyExternalIDFiles(
			bytes.NewReader(offsets), int64(len(offsets)),
			bytes.NewReader(data), int64(len(data)),
			1,
		); err == nil {
			t.Fatal("verifyExternalIDFiles() error = nil")
		}
	})

	t.Run("offset checksum", func(t *testing.T) {
		offsets, data := writeExternalIDTestFiles(t, []uint64{0, 1, 3}, "abc")
		offsets[fileHeaderBytes+documentOffsetBytes] = 2
		if err := verifyExternalIDFiles(
			bytes.NewReader(offsets), int64(len(offsets)),
			bytes.NewReader(data), int64(len(data)),
			2,
		); err == nil {
			t.Fatal("verifyExternalIDFiles() error = nil")
		}
	})

	t.Run("data checksum", func(t *testing.T) {
		offsets, data := writeExternalIDTestFiles(t, []uint64{0, 1, 3}, "abc")
		data[fileHeaderBytes] = 'z'
		if err := verifyExternalIDFiles(
			bytes.NewReader(offsets), int64(len(offsets)),
			bytes.NewReader(data), int64(len(data)),
			2,
		); err == nil {
			t.Fatal("verifyExternalIDFiles() error = nil")
		}
	})
}

func TestReadExternalIDs(t *testing.T) {
	offsets := encodeDocumentOffsets(t, 0, 1, 2)
	var got []string
	if err := readExternalIDs(
		bytes.NewReader(offsets),
		bytes.NewBufferString("ab"),
		2,
		uint64(len(offsets)),
		2,
		func(externalID string) error {
			got = append(got, externalID)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("external IDs = %q, want %q", got, want)
	}
}

func TestReadExternalIDsEmpty(t *testing.T) {
	offsets := encodeDocumentOffsets(t, 0)
	if err := readExternalIDs(
		bytes.NewReader(offsets),
		bytes.NewReader(nil),
		0,
		uint64(len(offsets)),
		0,
		func(string) error { return errors.New("unexpected external ID") },
	); err != nil {
		t.Fatal(err)
	}
}

func TestReadExternalIDsRejectsInvalidOffsets(t *testing.T) {
	tests := []struct {
		name          string
		offsets       []byte
		documentCount uint64
		offsetBytes   uint64
		data          string
		dataBytes     uint64
	}{
		{
			name:          "wrong offset body length",
			offsets:       encodeDocumentOffsets(t, 0, 1),
			documentCount: 2,
			offsetBytes:   16,
		},
		{
			name:          "first offset is not zero",
			offsets:       encodeDocumentOffsets(t, 1, 2),
			documentCount: 1,
			offsetBytes:   16,
			data:          "ab",
			dataBytes:     2,
		},
		{
			name:          "offsets are not increasing",
			offsets:       encodeDocumentOffsets(t, 0, 1, 1),
			documentCount: 2,
			offsetBytes:   24,
			data:          "a",
			dataBytes:     1,
		},
		{
			name:          "offset is outside data",
			offsets:       encodeDocumentOffsets(t, 0, 2),
			documentCount: 1,
			offsetBytes:   16,
			data:          "a",
			dataBytes:     1,
		},
		{
			name:          "final offset does not cover data",
			offsets:       encodeDocumentOffsets(t, 0, 1),
			documentCount: 1,
			offsetBytes:   16,
			data:          "a",
			dataBytes:     2,
		},
		{
			name:          "truncated offsets",
			offsets:       encodeDocumentOffsets(t, 0),
			documentCount: 1,
			offsetBytes:   16,
			data:          "a",
			dataBytes:     1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := readExternalIDs(
				bytes.NewReader(test.offsets),
				bytes.NewBufferString(test.data),
				test.documentCount,
				test.offsetBytes,
				test.dataBytes,
				func(string) error { return nil },
			); err == nil {
				t.Fatal("readExternalIDs() error = nil")
			}
		})
	}
}

func encodeDocumentOffsets(t *testing.T, offsets ...uint64) []byte {
	t.Helper()
	var encoded bytes.Buffer
	for _, offset := range offsets {
		if err := writeDocumentOffset(&encoded, offset); err != nil {
			t.Fatal(err)
		}
	}
	return encoded.Bytes()
}

func writeExternalIDTestFiles(t *testing.T, offsets []uint64, data string) ([]byte, []byte) {
	t.Helper()
	return writeIndexFileTestData(t, documentOffsetsRole, encodeDocumentOffsets(t, offsets...)),
		writeIndexFileTestData(t, documentDataRole, []byte(data))
}
