package indexfile

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyGoldenIndex(t *testing.T) {
	if err := Verify(context.Background(), goldenIndexDirectory); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGoldenIndexRejectsTruncation(t *testing.T) {
	for _, name := range goldenIndexFileNames {
		t.Run(name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			for size := len(readGoldenIndexFile(t, name)) - 1; size >= 0; size-- {
				if err := os.Truncate(filepath.Join(directory, name), int64(size)); err != nil {
					t.Fatal(err)
				}
				if err := Verify(context.Background(), directory); err == nil {
					t.Fatalf("Verify() error = nil after truncating %s to %d bytes", name, size)
				}
			}
		})
	}
}

func TestVerifyGoldenIndexRejectsBitFlip(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		offset int
		mask   byte
	}{
		{"file role", TermsFileName, 0, 1},
		{"format version", TermsFileName, 7, 1},
		{"metadata body", MetadataFileName, 24, 1},
		{"term body", TermsFileName, 28, 1},
		{"posting body", PostingsFileName, 20, 1},
		{"document-length body", DocumentLengthsFileName, 8, 1},
		{"document-offset body", DocumentOffsetsFileName, 16, 1},
		{"external-ID body", DocumentDataFileName, 8, 1},
		{"file footer", TermsFileName, 59, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			mutateGoldenIndexFile(t, directory, test.file, func(data []byte) {
				data[test.offset] ^= test.mask
			})
			if err := Verify(context.Background(), directory); err == nil {
				t.Fatal("Verify() error = nil")
			}
		})
	}
}

func TestVerifyGoldenIndexRejectsChecksumValidMetadataMutation(t *testing.T) {
	tests := []struct {
		name   string
		offset int
	}{
		{"analyzer ID", 8},
		{"postings codec ID", 12},
		{"manifest file length", 16},
		{"manifest file checksum", 24},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			mutateChecksummedGoldenFile(t, directory, MetadataFileName, func(data []byte) {
				data[test.offset] ^= 1
			})
			if err := Verify(context.Background(), directory); err == nil {
				t.Fatal("Verify() error = nil")
			}
		})
	}
}

func TestVerifyGoldenIndexRejectsChecksumValidDataMutation(t *testing.T) {
	tests := []struct {
		name                   string
		file                   string
		offset                 int
		mask                   byte
		manifestChecksumOffset int
	}{
		{"term length", TermsFileName, 8, 1, 24},
		{"document frequency", TermsFileName, 12, 1, 24},
		{"postings length", TermsFileName, 20, 1, 24},
		{"term UTF-8", TermsFileName, 28, 0x80, 24},
		{"term order", TermsFileName, 28, 0x10, 24},
		{"block endpoint", PostingsFileName, 8, 1, 36},
		{"block payload length", PostingsFileName, 12, 1, 36},
		{"posting document ID", PostingsFileName, 16, 1, 36},
		{"posting frequency", PostingsFileName, 20, 1, 36},
		{"document length", DocumentLengthsFileName, 8, 1, 48},
		{"initial document offset", DocumentOffsetsFileName, 8, 1, 60},
		{"intermediate document offset", DocumentOffsetsFileName, 16, 1, 60},
		{"final document offset", DocumentOffsetsFileName, 24, 1, 60},
		{"external-ID UTF-8", DocumentDataFileName, 8, 0x80, 72},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := copyGoldenIndex(t)
			checksum := mutateChecksummedGoldenFile(t, directory, test.file, func(data []byte) {
				data[test.offset] ^= test.mask
			})
			mutateChecksummedGoldenFile(t, directory, MetadataFileName, func(data []byte) {
				binary.LittleEndian.PutUint32(
					data[test.manifestChecksumOffset:test.manifestChecksumOffset+4],
					checksum,
				)
			})

			if _, err := verifyIndexStructure(directory); err != nil {
				t.Fatalf("checksum-valid mutation failed structural verification: %v", err)
			}
			if err := Verify(context.Background(), directory); err == nil {
				t.Fatal("Verify() error = nil")
			}
		})
	}
}

const goldenIndexDirectory = "testdata/golden-v1"

var goldenIndexFileNames = [...]string{
	MetadataFileName,
	TermsFileName,
	PostingsFileName,
	DocumentLengthsFileName,
	DocumentOffsetsFileName,
	DocumentDataFileName,
}

func readGoldenIndexFile(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(goldenIndexDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyGoldenIndex(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.CopyFS(directory, os.DirFS(goldenIndexDirectory)); err != nil {
		t.Fatal(err)
	}
	return directory
}

func mutateGoldenIndexFile(
	t *testing.T,
	directory string,
	name string,
	mutate func([]byte),
) {
	t.Helper()
	path := filepath.Join(directory, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutate(data)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mutateChecksummedGoldenFile(
	t *testing.T,
	directory string,
	name string,
	mutate func([]byte),
) uint32 {
	t.Helper()
	var checksum uint32
	mutateGoldenIndexFile(t, directory, name, func(data []byte) {
		mutate(data)
		checksum = crc32.Checksum(data[:len(data)-fileFooterBytes], crc32cTable)
		binary.LittleEndian.PutUint32(data[len(data)-fileFooterBytes:], checksum)
	})
	return checksum
}
