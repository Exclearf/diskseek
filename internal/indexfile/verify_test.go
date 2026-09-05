package indexfile

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestVerifyEmptyIndex(t *testing.T) {
	directory := writeVerificationTestIndex(t, nil, nil)
	if err := Verify(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsCrossFileMismatch(t *testing.T) {
	t.Run("document lengths and postings", func(t *testing.T) {
		directory := writeVerificationTestIndex(t, []index.DocumentMeta{
			{ExternalID: "a", Length: 1},
			{ExternalID: "b", Length: 4},
		}, verificationTestTerms())
		if err := Verify(context.Background(), directory); err == nil {
			t.Fatal("Verify() error = nil")
		}
	})

	t.Run("document lengths and external IDs", func(t *testing.T) {
		directory := writeVerificationTestIndex(t, []index.DocumentMeta{
			{ExternalID: "a", Length: 2},
			{ExternalID: "b", Length: 3},
		}, verificationTestTerms())
		replaceExternalIDFiles(t, directory, []index.DocumentMeta{{ExternalID: "a"}})
		if err := Verify(context.Background(), directory); err == nil {
			t.Fatal("Verify() error = nil")
		}
	})
}

func TestVerifyCancellation(t *testing.T) {
	directory := writeVerificationTestIndex(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Verify(ctx, directory); !errors.Is(err, context.Canceled) {
		t.Fatalf("Verify() error = %v, want context.Canceled", err)
	}
}

func verificationTestTerms() []termTestTerm {
	return []termTestTerm{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
		},
		{
			term:     "search",
			postings: []index.Posting{{DocumentID: 0, Frequency: 1}},
		},
	}
}

func writeVerificationTestIndex(
	t *testing.T,
	documents []index.DocumentMeta,
	terms []termTestTerm,
) string {
	t.Helper()

	var lengths, offsets, data bytes.Buffer
	nextDocument := 0
	documentMetadata, err := WriteDocumentFiles(&lengths, &offsets, &data, func() (index.DocumentMeta, error) {
		if nextDocument == len(documents) {
			return index.DocumentMeta{}, io.EOF
		}
		document := documents[nextDocument]
		nextDocument++
		return document, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var termData, postingData bytes.Buffer
	source := termTestSource{terms: terms}
	termMetadata, err := WriteTermFiles(
		&termData,
		&postingData,
		PostingsCodecRaw,
		source.nextTerm,
		source.nextPosting,
	)
	if err != nil {
		t.Fatal(err)
	}

	var metadata bytes.Buffer
	if err := WriteMetadataFile(&metadata, termMetadata, documentMetadata); err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	files := map[string][]byte{
		"index.meta":   metadata.Bytes(),
		"terms.bin":    termData.Bytes(),
		"postings.bin": postingData.Bytes(),
		"doclens.bin":  lengths.Bytes(),
		"docids.off":   offsets.Bytes(),
		"docids.dat":   data.Bytes(),
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func replaceExternalIDFiles(t *testing.T, directory string, documents []index.DocumentMeta) {
	t.Helper()

	var offsets, data bytes.Buffer
	nextDocument := 0
	replacement, err := WriteDocumentFiles(io.Discard, &offsets, &data, func() (index.DocumentMeta, error) {
		if nextDocument == len(documents) {
			return index.DocumentMeta{}, io.EOF
		}
		document := documents[nextDocument]
		nextDocument++
		return document, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	metadata, err := readIndexMetadataFile(filepath.Join(directory, "index.meta"))
	if err != nil {
		t.Fatal(err)
	}
	metadata.Documents.Offsets = replacement.Offsets
	metadata.Documents.Data = replacement.Data
	var encodedMetadata bytes.Buffer
	if err := WriteMetadataFile(&encodedMetadata, metadata.Terms, metadata.Documents); err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"index.meta": encodedMetadata.Bytes(),
		"docids.off": offsets.Bytes(),
		"docids.dat": data.Bytes(),
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
