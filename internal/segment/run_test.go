package segment

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestRunHeaderRoundTripAndBytes(t *testing.T) {
	var output bytes.Buffer
	header := runHeader{documentCount: documentIDLimit}
	if err := writeRunHeader(&output, header); err != nil {
		t.Fatal(err)
	}

	const want = "44534b52554e3031000000000000000001000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("header bytes = %s, want %s", got, want)
	}

	got, err := readRunHeader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got != header {
		t.Fatalf("readRunHeader() = %+v, want %+v", got, header)
	}
}

func TestReadRunHeaderRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "wrong magic", data: make([]byte, runHeaderBytes)},
		{name: "truncated header", data: []byte(runMagic)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readRunHeader(bytes.NewReader(test.data)); err == nil {
				t.Fatal("readRunHeader() error = nil")
			}
		})
	}
}

func TestRunTermHeaderRoundTripAndBytes(t *testing.T) {
	var output bytes.Buffer
	header := runHeader{documentCount: documentIDLimit}
	if err := writeRunTermHeader(&output, header, "go", documentIDLimit); err != nil {
		t.Fatal(err)
	}

	const want = "02000000676f0000000001000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("term bytes = %s, want %s", got, want)
	}

	term, postingCount, err := readRunTermHeader(bytes.NewReader(output.Bytes()), header)
	if err != nil {
		t.Fatal(err)
	}
	if term != "go" || postingCount != documentIDLimit {
		t.Fatalf("readRunTermHeader() = %q, %d; want %q, %d", term, postingCount, "go", documentIDLimit)
	}
}

func TestWriteRunTermHeaderRejectsInvalidValues(t *testing.T) {
	header := runHeader{documentCount: 1}
	tests := []struct {
		name         string
		term         string
		postingCount uint64
	}{
		{name: "empty term", postingCount: 1},
		{name: "invalid UTF-8", term: string([]byte{0xff}), postingCount: 1},
		{name: "oversized term", term: strings.Repeat("a", maxRunTermBytes+1), postingCount: 1},
		{name: "zero posting count", term: "a"},
		{name: "posting count above document count", term: "a", postingCount: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := writeRunTermHeader(io.Discard, header, test.term, test.postingCount); err == nil {
				t.Fatal("writeRunTermHeader() error = nil")
			}
		})
	}
}

func TestReadRunTermHeaderRejectsInvalidData(t *testing.T) {
	var oversized [4]byte
	binary.LittleEndian.PutUint32(oversized[:], maxRunTermBytes+1)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "oversized term", data: oversized[:]},
		{name: "truncated term", data: []byte{2, 0, 0, 0, 'g'}},
		{name: "invalid UTF-8", data: []byte{1, 0, 0, 0, 0xff}},
		{name: "truncated posting count", data: []byte{1, 0, 0, 0, 'a'}},
		{name: "zero posting count", data: append([]byte{1, 0, 0, 0, 'a'}, make([]byte, 8)...)},
		{name: "posting count above document count", data: append([]byte{1, 0, 0, 0, 'a', 2}, make([]byte, 7)...)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := readRunTermHeader(bytes.NewReader(test.data), runHeader{documentCount: 1}); err == nil {
				t.Fatal("readRunTermHeader() error = nil")
			}
		})
	}
}

func TestReadRunTermHeaderEndMarker(t *testing.T) {
	if _, _, err := readRunTermHeader(bytes.NewReader(make([]byte, 4)), runHeader{}); !errors.Is(err, io.EOF) {
		t.Fatalf("readRunTermHeader(end marker) error = %v, want EOF", err)
	}

	invalid := [][]byte{
		nil,
		{0, 0, 0, 0, 1},
	}
	for _, data := range invalid {
		if _, _, err := readRunTermHeader(bytes.NewReader(data), runHeader{}); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("readRunTermHeader(%v) error = %v, want non-EOF error", data, err)
		}
	}
}

func TestRunPostingRoundTripAndBytes(t *testing.T) {
	var output bytes.Buffer
	posting := index.Posting{DocumentID: 0x01020304, Frequency: 0x05060708}
	header := runHeader{firstDocumentID: posting.DocumentID, documentCount: 1}
	if err := writeRunPosting(&output, header, posting); err != nil {
		t.Fatal(err)
	}

	const want = "0403020108070605"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("posting bytes = %s, want %s", got, want)
	}

	got, err := readRunPosting(bytes.NewReader(output.Bytes()), header)
	if err != nil {
		t.Fatal(err)
	}
	if got != posting {
		t.Fatalf("readRunPosting() = %+v, want %+v", got, posting)
	}
}

func TestWriteRunPostingRejectsInvalidValues(t *testing.T) {
	header := runHeader{firstDocumentID: 7, documentCount: 1}
	tests := []index.Posting{
		{DocumentID: 7},
		{DocumentID: 8, Frequency: 1},
	}

	for _, posting := range tests {
		if err := writeRunPosting(io.Discard, header, posting); err == nil {
			t.Errorf("writeRunPosting(%+v) error = nil", posting)
		}
	}
}

func TestReadRunPostingRejectsInvalidData(t *testing.T) {
	header := runHeader{firstDocumentID: 7, documentCount: 1}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "truncated posting", data: []byte{7, 0, 0, 0, 1, 0, 0}},
		{name: "zero frequency", data: []byte{7, 0, 0, 0, 0, 0, 0, 0}},
		{name: "document outside run", data: []byte{8, 0, 0, 0, 1, 0, 0, 0}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readRunPosting(bytes.NewReader(test.data), header); err == nil {
				t.Fatal("readRunPosting() error = nil")
			}
		})
	}
}

func TestRunWriterBytes(t *testing.T) {
	output := &bufferWriteCloser{}
	writer, err := newRunWriter(output, runHeader{firstDocumentID: 7, documentCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.writeTerm("go", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.writePosting(index.Posting{DocumentID: 7, Frequency: 3}); err != nil {
		t.Fatal(err)
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}

	const want = "44534b52554e303107000000010000000000000002000000676f0100000000000000070000000300000000000000"
	if got := hex.EncodeToString(output.Bytes()); got != want {
		t.Fatalf("run bytes = %s, want %s", got, want)
	}
}

func TestRunWriterCloseReportsFlushAndCloseErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	output := &failingWriteCloser{writeErr: writeErr, closeErr: closeErr}
	writer, err := newRunWriter(output, runHeader{})
	if err != nil {
		t.Fatal(err)
	}

	err = writer.close()
	if !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("close() error = %v, want write and close errors", err)
	}
}

func TestRunWriterRejectsPostingCountMismatch(t *testing.T) {
	t.Run("next term before current term is complete", func(t *testing.T) {
		writer := newTestRunWriter(t, runHeader{documentCount: 2})
		if err := writer.writeTerm("a", 2); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{Frequency: 1}); err != nil {
			t.Fatal(err)
		}
		if err := writer.writeTerm("b", 1); err == nil {
			t.Fatal("writeTerm() error = nil")
		}
	})

	t.Run("more postings than declared", func(t *testing.T) {
		writer := newTestRunWriter(t, runHeader{documentCount: 1})
		if err := writer.writeTerm("a", 1); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{Frequency: 1}); err != nil {
			t.Fatal(err)
		}
		writeErr := writer.writePosting(index.Posting{Frequency: 1})
		if writeErr == nil {
			t.Fatal("writePosting() error = nil")
		}
		if err := writer.close(); !errors.Is(err, writeErr) {
			t.Fatalf("close() error = %v, want prior write error", err)
		}
	})

	t.Run("fewer postings than declared", func(t *testing.T) {
		writer := newTestRunWriter(t, runHeader{documentCount: 1})
		if err := writer.writeTerm("a", 1); err != nil {
			t.Fatal(err)
		}
		if err := writer.close(); err == nil {
			t.Fatal("close() error = nil")
		}
	})
}

func TestRunWriterRejectsOutOfOrderRecords(t *testing.T) {
	t.Run("terms", func(t *testing.T) {
		writer := newTestRunWriter(t, runHeader{documentCount: 1})
		if err := writer.writeTerm("b", 1); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{Frequency: 1}); err != nil {
			t.Fatal(err)
		}
		if err := writer.writeTerm("a", 1); err == nil {
			t.Fatal("writeTerm() error = nil")
		}
	})

	t.Run("postings", func(t *testing.T) {
		writer := newTestRunWriter(t, runHeader{firstDocumentID: 7, documentCount: 2})
		if err := writer.writeTerm("a", 2); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{DocumentID: 7, Frequency: 1}); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{DocumentID: 7, Frequency: 1}); err == nil {
			t.Fatal("writePosting() error = nil")
		}
	})
}

func TestRunWriterResetsDocumentOrderForEachTerm(t *testing.T) {
	writer := newTestRunWriter(t, runHeader{firstDocumentID: 7, documentCount: 1})
	for _, term := range []string{"a", "b"} {
		if err := writer.writeTerm(term, 1); err != nil {
			t.Fatal(err)
		}
		if err := writer.writePosting(index.Posting{DocumentID: 7, Frequency: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}
}

func TestRunWriterRejectsWriteAfterClose(t *testing.T) {
	writer := newTestRunWriter(t, runHeader{documentCount: 1})
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.writeTerm("a", 1); err == nil {
		t.Fatal("writeTerm() error = nil")
	}
}

func TestRunHeaderDocumentInterval(t *testing.T) {
	header := runHeader{firstDocumentID: 7, documentCount: 3}
	if err := validateRunHeader(header); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		documentID index.DocumentID
		want       bool
	}{
		{documentID: 6},
		{documentID: 7, want: true},
		{documentID: 9, want: true},
		{documentID: 10},
	}
	for _, test := range tests {
		if got := documentInRun(header, test.documentID); got != test.want {
			t.Errorf("documentInRun(%d) = %t, want %t", test.documentID, got, test.want)
		}
	}
}

func newTestRunWriter(t *testing.T, header runHeader) *runWriter {
	t.Helper()
	writer, err := newRunWriter(&bufferWriteCloser{}, header)
	if err != nil {
		t.Fatal(err)
	}
	return writer
}

type bufferWriteCloser struct {
	bytes.Buffer
}

func (*bufferWriteCloser) Close() error {
	return nil
}

type failingWriteCloser struct {
	writeErr error
	closeErr error
}

func (w *failingWriteCloser) Write([]byte) (int, error) {
	return 0, w.writeErr
}

func (w *failingWriteCloser) Close() error {
	return w.closeErr
}

func TestRunHeaderRejectsInvalidInterval(t *testing.T) {
	tests := []runHeader{
		{firstDocumentID: 1},
		{firstDocumentID: index.DocumentID(documentIDLimit - 1), documentCount: 2},
	}

	for _, header := range tests {
		if err := validateRunHeader(header); err == nil {
			t.Errorf("validateRunHeader(%+v) error = nil", header)
		}
	}
}
