package indexfile

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestWriteDocumentBodies(t *testing.T) {
	documents := []index.DocumentMeta{
		{ExternalID: "a", Length: 2},
		{ExternalID: "b", Length: 3},
	}

	var lengths, offsets, data bytes.Buffer
	next := 0
	if err := writeDocumentBodies(&lengths, &offsets, &data, func() (index.DocumentMeta, error) {
		if next == len(documents) {
			return index.DocumentMeta{}, io.EOF
		}
		document := documents[next]
		next++
		return document, nil
	}); err != nil {
		t.Fatal(err)
	}

	got := make([]index.DocumentMeta, len(documents))
	for position := range got {
		length, err := readDocumentLength(&lengths)
		if err != nil {
			t.Fatal(err)
		}
		got[position].Length = length
	}
	if lengths.Len() != 0 {
		t.Fatalf("unread document-length bytes = %d", lengths.Len())
	}

	offsetBytes := uint64(offsets.Len())
	dataBytes := uint64(data.Len())
	next = 0
	if err := readExternalIDs(&offsets, &data, uint64(len(documents)), offsetBytes, dataBytes, func(externalID string) error {
		got[next].ExternalID = externalID
		next++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, documents) {
		t.Fatalf("documents = %#v, want %#v", got, documents)
	}
}

func TestWriteDocumentBodiesEmpty(t *testing.T) {
	var lengths, offsets, data bytes.Buffer
	if err := writeDocumentBodies(&lengths, &offsets, &data, func() (index.DocumentMeta, error) {
		return index.DocumentMeta{}, io.EOF
	}); err != nil {
		t.Fatal(err)
	}
	if lengths.Len() != 0 || data.Len() != 0 {
		t.Fatal("empty documents produced length or external-ID data")
	}

	offsetBytes := uint64(offsets.Len())
	if err := readExternalIDs(
		&offsets,
		&data,
		0,
		offsetBytes,
		0,
		func(string) error { return errors.New("unexpected external ID") },
	); err != nil {
		t.Fatal(err)
	}
}

func TestWriteDocumentFiles(t *testing.T) {
	documents := []index.DocumentMeta{
		{ExternalID: "a", Length: 2},
		{ExternalID: "b", Length: 3},
	}

	var lengths, offsets, data bytes.Buffer
	next := 0
	metadata, err := WriteDocumentFiles(&lengths, &offsets, &data, func() (index.DocumentMeta, error) {
		if next == len(documents) {
			return index.DocumentMeta{}, io.EOF
		}
		document := documents[next]
		next++
		return document, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		got      []byte
		want     string
		metadata FileMetadata
	}{
		{
			name: "document lengths",
			got:  lengths.Bytes(),
			want: "DSKDLEN\x01\x02\x00\x00\x00\x03\x00\x00\x00\xd4\x8a\xe0\x00",
			metadata: FileMetadata{
				Length:   20,
				Checksum: 0x00e08ad4,
			},
		},
		{
			name: "document offsets",
			got:  offsets.Bytes(),
			want: "DSKDOFF\x01" +
				"\x00\x00\x00\x00\x00\x00\x00\x00" +
				"\x01\x00\x00\x00\x00\x00\x00\x00" +
				"\x02\x00\x00\x00\x00\x00\x00\x00" +
				"\x1b\x8a\xed\xfe",
			metadata: FileMetadata{
				Length:   36,
				Checksum: 0xfeed8a1b,
			},
		},
		{
			name: "document data",
			got:  data.Bytes(),
			want: "DSKDDAT\x01ab\x02\x66\x22\x20",
			metadata: FileMetadata{
				Length:   14,
				Checksum: 0x20226602,
			},
		},
	}
	gotMetadata := []FileMetadata{metadata.Lengths, metadata.Offsets, metadata.Data}
	for position, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !bytes.Equal(test.got, []byte(test.want)) {
				t.Fatalf("file = % x, want % x", test.got, test.want)
			}
			if gotMetadata[position] != test.metadata {
				t.Fatalf("metadata = %+v, want %+v", gotMetadata[position], test.metadata)
			}
		})
	}
}
