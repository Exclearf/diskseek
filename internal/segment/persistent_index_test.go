package segment

import (
	"bytes"
	"io"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestWritePersistentDocumentFilesReadsSidecar(t *testing.T) {
	documents := []index.DocumentMeta{
		{ExternalID: "a", Length: 2},
		{ExternalID: "b", Length: 3},
	}

	var gotLengths, gotOffsets, gotData bytes.Buffer
	gotMetadata, err := writePersistentDocumentFiles(
		bytes.NewReader(encodeDocuments(t, documents)),
		&gotLengths,
		&gotOffsets,
		&gotData,
	)
	if err != nil {
		t.Fatal(err)
	}

	var wantLengths, wantOffsets, wantData bytes.Buffer
	next := 0
	wantMetadata, err := indexfile.WriteDocumentFiles(
		&wantLengths,
		&wantOffsets,
		&wantData,
		func() (index.DocumentMeta, error) {
			if next == len(documents) {
				return index.DocumentMeta{}, io.EOF
			}
			document := documents[next]
			next++
			return document, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(gotLengths.Bytes(), wantLengths.Bytes()) ||
		!bytes.Equal(gotOffsets.Bytes(), wantOffsets.Bytes()) ||
		!bytes.Equal(gotData.Bytes(), wantData.Bytes()) {
		t.Fatal("sidecar-driven document files do not match direct encoding")
	}
	if gotMetadata != wantMetadata {
		t.Fatalf("metadata = %+v, want %+v", gotMetadata, wantMetadata)
	}
}
