package indexfile

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

var (
	goldenTermMetadata = TermFilesMetadata{
		Terms:    FileMetadata{Length: 60, Checksum: 0xfd50af02},
		Postings: FileMetadata{Length: 52, Checksum: 0x3d5463ec},
	}
	goldenDocumentMetadata = DocumentFilesMetadata{
		Lengths: FileMetadata{Length: 20, Checksum: 0x00e08ad4},
		Offsets: FileMetadata{Length: 36, Checksum: 0xfeed8a1b},
		Data:    FileMetadata{Length: 14, Checksum: 0x20226602},
	}
)

func TestReadMetadataFile(t *testing.T) {
	metadata, err := readMetadataFile(bytes.NewReader([]byte(goldenMetadataFile)), int64(len(goldenMetadataFile)))
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

func TestReadMetadataFileRejectsInvalidData(t *testing.T) {
	corruptChecksum := []byte(goldenMetadataFile)
	corruptChecksum[len(corruptChecksum)-1] ^= 1
	tests := []struct {
		name string
		data []byte
	}{
		{"short file", []byte(goldenMetadataFile[:len(goldenMetadataFile)-1])},
		{"long file", append([]byte(goldenMetadataFile), 0)},
		{"wrong checksum", corruptChecksum},
		{"unsupported analyzer", mutateMetadata(func(data []byte) {
			binary.LittleEndian.PutUint32(data[8:12], analyzerContractID+1)
		})},
		{"unsupported codec", mutateMetadata(func(data []byte) {
			binary.LittleEndian.PutUint32(data[12:16], rawPostingsCodecID+1)
		})},
		{"terms file below minimum", mutateMetadata(func(data []byte) {
			binary.LittleEndian.PutUint64(data[16:24], fileHeaderBytes+fileFooterBytes-1)
		})},
		{"offset file below minimum", mutateMetadata(func(data []byte) {
			binary.LittleEndian.PutUint64(data[52:60], fileHeaderBytes+fileFooterBytes)
		})},
		{"file above maximum", mutateMetadata(func(data []byte) {
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

func mutateMetadata(mutate func([]byte)) []byte {
	data := []byte(goldenMetadataFile)
	mutate(data)
	checksum := crc32.Checksum(data[:len(data)-fileFooterBytes], crc32cTable)
	binary.LittleEndian.PutUint32(data[len(data)-fileFooterBytes:], checksum)
	return data
}

const goldenMetadataFile = "DSKMETA\x01" +
	"\x01\x00\x00\x00\x01\x00\x00\x00" +
	"\x3c\x00\x00\x00\x00\x00\x00\x00\x02\xaf\x50\xfd" +
	"\x34\x00\x00\x00\x00\x00\x00\x00\xec\x63\x54\x3d" +
	"\x14\x00\x00\x00\x00\x00\x00\x00\xd4\x8a\xe0\x00" +
	"\x24\x00\x00\x00\x00\x00\x00\x00\x1b\x8a\xed\xfe" +
	"\x0e\x00\x00\x00\x00\x00\x00\x00\x02\x66\x22\x20" +
	"\xca\x54\x76\xe2"

func TestWriteMetadataFile(t *testing.T) {

	var output bytes.Buffer
	if err := WriteMetadataFile(&output, goldenTermMetadata, goldenDocumentMetadata); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(output.Bytes(), []byte(goldenMetadataFile)) {
		t.Fatalf("metadata file = % x, want % x", output.Bytes(), goldenMetadataFile)
	}
}
