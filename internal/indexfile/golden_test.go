package indexfile

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestVerifyGoldenIndexes(t *testing.T) {
	for _, directory := range []string{goldenIndexDirectory, goldenVByteIndexDirectory} {
		t.Run(filepath.Base(directory), func(t *testing.T) {
			if err := Verify(context.Background(), directory); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGoldenIndexesHaveSamePostings(t *testing.T) {
	raw, err := Open(goldenIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := raw.Close(); err != nil {
			t.Error(err)
		}
	}()

	vbyte, err := Open(goldenVByteIndexDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := vbyte.Close(); err != nil {
			t.Error(err)
		}
	}()

	for _, term := range []string{"go", "search"} {
		rawPostings := collectPostings(t, raw, term)
		vbytePostings := collectPostings(t, vbyte, term)
		if !slices.Equal(rawPostings, vbytePostings) {
			t.Fatalf("%q postings: raw = %v, vbyte = %v", term, rawPostings, vbytePostings)
		}
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
		{"term body", TermsFileName, 32, 1},
		{"posting body", PostingsFileName, 20, 1},
		{"document-length body", DocumentLengthsFileName, 8, 1},
		{"document-offset body", DocumentOffsetsFileName, 16, 1},
		{"external-ID body", DocumentDataFileName, 8, 1},
		{"file footer", TermsFileName, 67, 1},
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
		{"maximum term frequency", TermsFileName, 28, 1, 24},
		{"term UTF-8", TermsFileName, 32, 0x80, 24},
		{"term order", TermsFileName, 32, 0x10, 24},
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

func TestVerifyVByteGoldenIndexRejectsChecksumValidZeroGap(t *testing.T) {
	directory := copyGoldenIndexFrom(t, goldenVByteIndexDirectory)
	checksum := mutateChecksummedGoldenFile(t, directory, PostingsFileName, func(data []byte) {
		data[fileHeaderBytes+postingBlockHeaderBytes+2] = 0x80
	})
	mutateChecksummedGoldenFile(t, directory, MetadataFileName, func(data []byte) {
		binary.LittleEndian.PutUint32(data[36:40], checksum)
	})

	if _, err := verifyIndexStructure(directory); err != nil {
		t.Fatalf("checksum-valid mutation failed structural verification: %v", err)
	}
	if err := Verify(context.Background(), directory); err == nil {
		t.Fatal("Verify() error = nil")
	}
}

const goldenIndexDirectory = "testdata/golden-v1/raw"
const goldenVByteIndexDirectory = "testdata/golden-v1/vbyte"

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
	return readGoldenIndexFileFrom(t, goldenIndexDirectory, name)
}

func readGoldenIndexFileFrom(t testing.TB, directory, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func copyGoldenIndex(t *testing.T) string {
	t.Helper()
	return copyGoldenIndexFrom(t, goldenIndexDirectory)
}

func copyGoldenIndexFrom(t *testing.T, source string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.CopyFS(directory, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return directory
}

func collectPostings(t *testing.T, opened *Index, term string) []index.Posting {
	t.Helper()
	cursor, found, err := opened.Postings(term)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("Postings(%q) found = false", term)
	}

	var postings []index.Posting
	for {
		posting, valid := cursor.Current()
		if !valid {
			return postings
		}
		postings = append(postings, posting)
		valid, err = cursor.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			return postings
		}
	}
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
