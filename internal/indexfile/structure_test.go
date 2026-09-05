package indexfile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestVerifyIndexStructure(t *testing.T) {
	directory, want := writeStructureTestIndex(t)
	metadata, err := verifyIndexStructure(directory)
	if err != nil {
		t.Fatal(err)
	}
	if metadata != want {
		t.Fatalf("metadata = %+v, want %+v", metadata, want)
	}
}

func TestVerifyIndexStructureRejectsMismatchedFile(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"physical length", func(data []byte) []byte { return append(data, 0) }},
		{"role", func(data []byte) []byte {
			data[0] ^= 1
			return data
		}},
		{"stored footer", func(data []byte) []byte {
			data[len(data)-1] ^= 1
			return data
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, _ := writeStructureTestIndex(t)
			path := filepath.Join(directory, "terms.bin")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, test.mutate(data), 0o600); err != nil {
				t.Fatal(err)
			}

			if _, err := verifyIndexStructure(directory); err == nil {
				t.Fatal("verifyIndexStructure() error = nil")
			}
		})
	}
}

func writeStructureTestIndex(t *testing.T) (string, indexMetadata) {
	t.Helper()

	var terms, postings bytes.Buffer
	source := termTestSource{}
	termMetadata, err := WriteTermFiles(&terms, &postings, source.nextTerm, source.nextPosting)
	if err != nil {
		t.Fatal(err)
	}

	var lengths, offsets, data bytes.Buffer
	documentMetadata, err := WriteDocumentFiles(&lengths, &offsets, &data, func() (index.DocumentMeta, error) {
		return index.DocumentMeta{}, io.EOF
	})
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
		"terms.bin":    terms.Bytes(),
		"postings.bin": postings.Bytes(),
		"doclens.bin":  lengths.Bytes(),
		"docids.off":   offsets.Bytes(),
		"docids.dat":   data.Bytes(),
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory, indexMetadata{Terms: termMetadata, Documents: documentMetadata}
}
