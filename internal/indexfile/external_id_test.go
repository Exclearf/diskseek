package indexfile

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func TestReadExternalID(t *testing.T) {
	got, err := readExternalID(strings.NewReader("source"), uint64(len("source")))
	if err != nil {
		t.Fatal(err)
	}
	if got != "source" {
		t.Fatalf("external ID = %q, want %q", got, "source")
	}
}

func TestReadExternalIDRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		length uint64
	}{
		{name: "empty"},
		{name: "oversized", length: corpus.MaxExternalIDBytes + 1},
		{name: "invalid UTF-8", data: []byte{0xff}, length: 1},
		{name: "truncated", data: []byte("a"), length: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readExternalID(bytes.NewReader(test.data), test.length); err == nil {
				t.Fatal("readExternalID() error = nil")
			}
		})
	}
}

func TestIndexExternalID(t *testing.T) {
	opened, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()

	for documentID, want := range []string{"a", "b"} {
		got, err := opened.ExternalID(index.DocumentID(documentID))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("ExternalID(%d) = %q, want %q", documentID, got, want)
		}
	}

	if _, err := opened.ExternalID(2); err == nil {
		t.Fatal("ExternalID(2) error = nil")
	}

	opened.documentOffsetsBodyBytes = documentOffsetBytes
	if _, err := opened.ExternalID(0); err == nil {
		t.Fatal("ExternalID(0) error = nil with missing end offset")
	}
}

func TestIndexExternalIDRejectsInvalidSelectedRange(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		documentID index.DocumentID
		mutate     func([]byte)
	}{
		{
			name:       "decreasing",
			file:       DocumentOffsetsFileName,
			documentID: 1,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint64(data[fileHeaderBytes+2*documentOffsetBytes:], 0)
			},
		},
		{
			name: "outside data",
			file: DocumentOffsetsFileName,
			mutate: func(data []byte) {
				binary.LittleEndian.PutUint64(data[fileHeaderBytes+documentOffsetBytes:], math.MaxUint64)
			},
		},
		{
			name: "invalid UTF-8",
			file: DocumentDataFileName,
			mutate: func(data []byte) {
				data[fileHeaderBytes] = 0xff
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			mutateGoldenIndexFile(t, directory, test.file, test.mutate)

			opened, err := Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := opened.Close(); err != nil {
					t.Error(err)
				}
			}()

			if _, err := opened.ExternalID(test.documentID); err == nil {
				t.Fatalf("ExternalID(%d) error = nil", test.documentID)
			}
		})
	}
}

func TestIndexExternalIDPropagatesShortRead(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(*Index)
	}{
		{
			name: "offsets",
			replace: func(opened *Index) {
				opened.documentOffsets = shortReadIndexFile{opened.documentOffsets}
			},
		},
		{
			name: "data",
			replace: func(opened *Index) {
				opened.documentData = shortReadIndexFile{opened.documentData}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			opened, err := Open(goldenIndexDirectory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := opened.Close(); err != nil {
					t.Error(err)
				}
			}()
			test.replace(opened)

			if _, err := opened.ExternalID(0); err == nil {
				t.Fatal("ExternalID(0) error = nil")
			}
		})
	}
}

func TestIndexExternalIDAfterClose(t *testing.T) {
	opened, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.ExternalID(0); err == nil {
		t.Fatal("ExternalID(0) error = nil after Close")
	}
}

type shortReadIndexFile struct {
	indexFile
}

func (f shortReadIndexFile) ReadAt(data []byte, _ int64) (int, error) {
	return len(data) - 1, io.EOF
}
