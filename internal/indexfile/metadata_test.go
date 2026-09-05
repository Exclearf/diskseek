package indexfile

import (
	"bytes"
	"testing"
)

func TestWriteMetadataFile(t *testing.T) {
	terms := TermFilesMetadata{
		Terms:    FileMetadata{Length: 60, Checksum: 0xfd50af02},
		Postings: FileMetadata{Length: 52, Checksum: 0x3d5463ec},
	}
	documents := DocumentFilesMetadata{
		Lengths: FileMetadata{Length: 20, Checksum: 0x00e08ad4},
		Offsets: FileMetadata{Length: 36, Checksum: 0xfeed8a1b},
		Data:    FileMetadata{Length: 14, Checksum: 0x20226602},
	}

	var output bytes.Buffer
	if err := WriteMetadataFile(&output, terms, documents); err != nil {
		t.Fatal(err)
	}

	want := "DSKMETA\x01" +
		"\x01\x00\x00\x00\x01\x00\x00\x00" +
		"\x3c\x00\x00\x00\x00\x00\x00\x00\x02\xaf\x50\xfd" +
		"\x34\x00\x00\x00\x00\x00\x00\x00\xec\x63\x54\x3d" +
		"\x14\x00\x00\x00\x00\x00\x00\x00\xd4\x8a\xe0\x00" +
		"\x24\x00\x00\x00\x00\x00\x00\x00\x1b\x8a\xed\xfe" +
		"\x0e\x00\x00\x00\x00\x00\x00\x00\x02\x66\x22\x20" +
		"\xca\x54\x76\xe2"
	if !bytes.Equal(output.Bytes(), []byte(want)) {
		t.Fatalf("metadata file = % x, want % x", output.Bytes(), want)
	}
}
