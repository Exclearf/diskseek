package indexfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"testing"
)

func TestFileReaderValidatesChecksum(t *testing.T) {
	data := []byte("DSKTERM\x01go\x00\x00\x00\x00")
	binary.LittleEndian.PutUint32(data[len(data)-fileFooterBytes:], crc32.Checksum(data[:len(data)-fileFooterBytes], crc32cTable))

	reader, err := newFileReader(bytes.NewReader(data), int64(len(data)), termsRole)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "go" {
		t.Fatalf("body = %q, want %q", body, "go")
	}
	if err := reader.finish(); err != nil {
		t.Fatal(err)
	}

	data[fileHeaderBytes] ^= 1
	reader, err = newFileReader(bytes.NewReader(data), int64(len(data)), termsRole)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.finish(); err == nil {
		t.Fatal("finish() error = nil after body corruption")
	}
}

func TestFileReaderRequiresCompleteBody(t *testing.T) {
	if _, err := newFileReader(bytes.NewReader(make([]byte, fileHeaderBytes+fileFooterBytes-1)), fileHeaderBytes+fileFooterBytes-1, termsRole); err == nil {
		t.Fatal("newFileReader() error = nil for undersized file")
	}

	data := []byte("DSKTERM\x01go\x00\x00\x00\x00")
	reader, err := newFileReader(bytes.NewReader(data), int64(len(data)), termsRole)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.finish(); err == nil {
		t.Fatal("finish() error = nil with unread body")
	}
}

func TestFileWriterChecksumCoverage(t *testing.T) {
	var output bytes.Buffer
	writer, err := newFileWriter(&output, termsRole)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "go"); err != nil {
		t.Fatal(err)
	}

	metadata, err := writer.finish()
	if err != nil {
		t.Fatal(err)
	}

	data := output.Bytes()
	bodyEnd := fileHeaderBytes + len("go")
	if len(data) != bodyEnd+fileFooterBytes {
		t.Fatalf("file length = %d, want %d", len(data), bodyEnd+fileFooterBytes)
	}
	if metadata.Length != uint64(len(data)) {
		t.Fatalf("reported file length = %d, want %d", metadata.Length, len(data))
	}
	if got := string(data[:bodyEnd]); got != "DSKTERM\x01go" {
		t.Fatalf("header and body = %q, want %q", got, "DSKTERM\x01go")
	}

	stored, err := readFooter(bytes.NewReader(data[bodyEnd:]))
	if err != nil {
		t.Fatal(err)
	}
	recomputed := crc32.Checksum(data[:bodyEnd], crc32cTable)
	if metadata.Checksum != stored || stored != recomputed {
		t.Fatalf("checksums: returned=%08x stored=%08x recomputed=%08x", metadata.Checksum, stored, recomputed)
	}
}

func TestFileWriterReturnsFlushError(t *testing.T) {
	want := errors.New("write failed")
	writer, err := newFileWriter(errorWriter{err: want}, termsRole)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.finish(); !errors.Is(err, want) {
		t.Fatalf("finish() error = %v, want %v", err, want)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
