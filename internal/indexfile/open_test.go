package indexfile

import (
	"context"
	"encoding/binary"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenGoldenIndex(t *testing.T) {
	opened, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := opened.Close(); err != nil {
			t.Error(err)
		}
	}()

	wantTerms := map[string]termEntry{
		"go": {
			documentFrequency: 2,
			postingsOffset:    8,
			postingsBytes:     24,
		},
		"search": {
			documentFrequency: 1,
			postingsOffset:    32,
			postingsBytes:     16,
		},
	}
	if !maps.Equal(opened.terms, wantTerms) {
		t.Fatalf("terms = %+v, want %+v", opened.terms, wantTerms)
	}
	if !slices.Equal(opened.documentLengths, []uint32{2, 3}) {
		t.Fatalf("document lengths = %v, want [2 3]", opened.documentLengths)
	}
	if opened.documentsWithTerms != 2 || opened.totalLength != 5 || opened.averageDocumentLength != 2.5 {
		t.Fatalf(
			"statistics = (%d, %d, %g), want (2, 5, 2.5)",
			opened.documentsWithTerms,
			opened.totalLength,
			opened.averageDocumentLength,
		)
	}
}

func TestOpenDoesNotScanLazyFiles(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		offset int
	}{
		{"postings", PostingsFileName, fileHeaderBytes + rawPostingBlockHeaderBytes},
		{"external ID offsets", DocumentOffsetsFileName, fileHeaderBytes},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			mutateGoldenIndexFile(t, directory, test.file, func(data []byte) {
				data[test.offset] ^= 1
			})

			opened, err := Open(directory)
			if err != nil {
				t.Fatal(err)
			}
			if err := opened.Close(); err != nil {
				t.Fatal(err)
			}
			if err := Verify(context.Background(), directory); err == nil {
				t.Fatal("Verify() error = nil")
			}
		})
	}
}

func TestOpenRejectsDocumentFrequencyAboveDocumentsWithTerms(t *testing.T) {
	directory := copyGoldenIndex(t)
	checksum := mutateChecksummedGoldenFile(t, directory, DocumentLengthsFileName, func(data []byte) {
		binary.LittleEndian.PutUint32(data[fileHeaderBytes+documentLengthBytes:], 0)
	})
	mutateChecksummedGoldenFile(t, directory, MetadataFileName, func(data []byte) {
		binary.LittleEndian.PutUint32(data[48:52], checksum)
	})

	if opened, err := Open(directory); err == nil {
		_ = opened.Close()
		t.Fatal("Open() error = nil")
	}
}

func TestOpenOwnsFileLifetimes(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		closeCalls := make(map[string]int)
		opened, err := openIndex(goldenIndexDirectory, func(path string) (indexFile, error) {
			return openTrackedIndexFile(path, closeCalls, nil)
		})
		if err != nil {
			t.Fatal(err)
		}

		want := map[string]int{
			MetadataFileName:        1,
			TermsFileName:           1,
			DocumentLengthsFileName: 1,
		}
		if !maps.Equal(closeCalls, want) {
			t.Fatalf("close calls = %v, want %v", closeCalls, want)
		}

		if err := opened.Close(); err != nil {
			t.Fatal(err)
		}
		want[PostingsFileName] = 1
		want[DocumentOffsetsFileName] = 1
		want[DocumentDataFileName] = 1
		if !maps.Equal(closeCalls, want) {
			t.Fatalf("close calls = %v, want %v", closeCalls, want)
		}
	})

	t.Run("failure", func(t *testing.T) {
		openErr := errors.New("open failed")
		closeErr := errors.New("close failed")
		closeCalls := make(map[string]int)
		_, err := openIndex(goldenIndexDirectory, func(path string) (indexFile, error) {
			name := filepath.Base(path)
			if name == DocumentDataFileName {
				return nil, openErr
			}
			var injected error
			if name == TermsFileName {
				injected = closeErr
			}
			return openTrackedIndexFile(path, closeCalls, injected)
		})
		if !errors.Is(err, openErr) || !errors.Is(err, closeErr) {
			t.Fatalf("openIndex() error = %v, want open and close errors", err)
		}
		want := map[string]int{
			MetadataFileName:        1,
			TermsFileName:           1,
			PostingsFileName:        1,
			DocumentLengthsFileName: 1,
			DocumentOffsetsFileName: 1,
		}
		if !maps.Equal(closeCalls, want) {
			t.Fatalf("close calls = %v, want %v", closeCalls, want)
		}
	})
}

type trackedIndexFile struct {
	*os.File
	name       string
	closeCalls map[string]int
	closeErr   error
}

func openTrackedIndexFile(
	path string,
	closeCalls map[string]int,
	closeErr error,
) (*trackedIndexFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &trackedIndexFile{
		File:       file,
		name:       filepath.Base(path),
		closeCalls: closeCalls,
		closeErr:   closeErr,
	}, nil
}

func (f *trackedIndexFile) Close() error {
	f.closeCalls[f.name]++
	return errors.Join(f.File.Close(), f.closeErr)
}
