package indexfile

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"testing"
)

var (
	goldenTermMetadata = TermFilesMetadata{
		Terms:    FileMetadata{Length: 60, Checksum: 0xfd50af02},
		Postings: FileMetadata{Length: 52, Checksum: 0x3d5463ec},
		Codec:    PostingsCodecRaw,
	}
	goldenVByteTermMetadata = TermFilesMetadata{
		Terms:    FileMetadata{Length: 60, Checksum: 0x5159752a},
		Postings: FileMetadata{Length: 34, Checksum: 0x8827a46a},
		Codec:    PostingsCodecVByte,
	}
	goldenDocumentMetadata = DocumentFilesMetadata{
		Lengths: FileMetadata{Length: 20, Checksum: 0x00e08ad4},
		Offsets: FileMetadata{Length: 36, Checksum: 0xfeed8a1b},
		Data:    FileMetadata{Length: 14, Checksum: 0x20226602},
	}
)

func TestWriteMetadataFiles(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		terms     TermFilesMetadata
	}{
		{"raw", goldenIndexDirectory, goldenTermMetadata},
		{"vbyte", goldenVByteIndexDirectory, goldenVByteTermMetadata},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := WriteMetadataFile(&output, test.terms, goldenDocumentMetadata); err != nil {
				t.Fatal(err)
			}

			want := readGoldenIndexFileFrom(t, test.directory, MetadataFileName)
			if !bytes.Equal(output.Bytes(), want) {
				t.Fatalf("metadata file = % x, want % x", output.Bytes(), want)
			}
		})
	}
}

func TestWriteMetadataFileRejectsUnsupportedPostingsCodec(t *testing.T) {
	terms := goldenTermMetadata
	terms.Codec = PostingsCodec(3)
	if err := WriteMetadataFile(io.Discard, terms, goldenDocumentMetadata); err == nil {
		t.Fatal("WriteMetadataFile() error = nil")
	}
}

func TestReadMetadataFile(t *testing.T) {
	data := readGoldenIndexFile(t, MetadataFileName)
	metadata, err := readMetadataFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want := indexMetadata{
		Terms:     goldenTermMetadata,
		Documents: goldenDocumentMetadata,
	}
	if metadata != want {
		t.Fatalf("metadata = %+v, want %+v", metadata, want)
	}
}

func TestReadVByteMetadataFile(t *testing.T) {
	data := readGoldenIndexFileFrom(t, goldenVByteIndexDirectory, MetadataFileName)
	metadata, err := readMetadataFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	want := indexMetadata{
		Terms:     goldenVByteTermMetadata,
		Documents: goldenDocumentMetadata,
	}
	if metadata != want {
		t.Fatalf("metadata = %+v, want %+v", metadata, want)
	}
}

func TestReadMetadataFileRejectsInvalidData(t *testing.T) {
	data := readGoldenIndexFile(t, MetadataFileName)
	corruptChecksum := bytes.Clone(data)
	corruptChecksum[len(corruptChecksum)-1] ^= 1
	tests := []struct {
		name string
		data []byte
	}{
		{"short file", data[:len(data)-1]},
		{"long file", append(bytes.Clone(data), 0)},
		{"wrong checksum", corruptChecksum},
		{"unsupported analyzer", mutateMetadata(t, func(data []byte) {
			binary.LittleEndian.PutUint32(data[8:12], analyzerContractID+1)
		})},
		{"unsupported codec", mutateMetadata(t, func(data []byte) {
			binary.LittleEndian.PutUint32(data[12:16], 3)
		})},
		{"terms file below minimum", mutateMetadata(t, func(data []byte) {
			binary.LittleEndian.PutUint64(data[16:24], fileHeaderBytes+fileFooterBytes-1)
		})},
		{"offset file below minimum", mutateMetadata(t, func(data []byte) {
			binary.LittleEndian.PutUint64(data[52:60], fileHeaderBytes+fileFooterBytes)
		})},
		{"file above maximum", mutateMetadata(t, func(data []byte) {
			binary.LittleEndian.PutUint64(data[64:72], maximumFileBytes+1)
		})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readMetadataFile(bytes.NewReader(test.data), int64(len(test.data))); err == nil {
				t.Fatal("readMetadataFile() error = nil")
			}
		})
	}
}

func mutateMetadata(t *testing.T, mutate func([]byte)) []byte {
	t.Helper()
	data := readGoldenIndexFile(t, MetadataFileName)
	mutate(data)
	checksum := crc32.Checksum(data[:len(data)-fileFooterBytes], crc32cTable)
	binary.LittleEndian.PutUint32(data[len(data)-fileFooterBytes:], checksum)
	return data
}
