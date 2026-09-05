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
