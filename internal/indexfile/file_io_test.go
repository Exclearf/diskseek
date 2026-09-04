package indexfile

import (
	"bytes"
	"hash/crc32"
	"io"
	"testing"
)

func TestFileWriterChecksumCoverage(t *testing.T) {
	var output bytes.Buffer
	writer, err := newFileWriter(&output, termsRole)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "go"); err != nil {
		t.Fatal(err)
	}

	checksum, err := writer.finish()
	if err != nil {
		t.Fatal(err)
	}

	data := output.Bytes()
	bodyEnd := fileHeaderBytes + len("go")
	if len(data) != bodyEnd+fileFooterBytes {
		t.Fatalf("file length = %d, want %d", len(data), bodyEnd+fileFooterBytes)
	}
	if got := string(data[:bodyEnd]); got != "DSKTERM\x01go" {
		t.Fatalf("header and body = %q, want %q", got, "DSKTERM\x01go")
	}

	stored, err := readFooter(bytes.NewReader(data[bodyEnd:]))
	if err != nil {
		t.Fatal(err)
	}
	recomputed := crc32.Checksum(data[:bodyEnd], crc32cTable)
	if checksum != stored || stored != recomputed {
		t.Fatalf("checksums: returned=%08x stored=%08x recomputed=%08x", checksum, stored, recomputed)
	}
}
