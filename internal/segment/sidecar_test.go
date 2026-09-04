package segment

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func TestDocumentSidecarRoundTripAndBytes(t *testing.T) {
	tests := []struct {
		name      string
		documents []index.DocumentMeta
		wantHex   string
	}{
		{name: "empty", wantHex: "44534b444f43303100000000"},
		{
			name: "documents",
			documents: []index.DocumentMeta{
				{ExternalID: "shared", Length: 2},
				{ExternalID: "shared", Length: 0},
			},
			wantHex: "44534b444f4330310600000073686172656402000000060000007368617265640000000000000000",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded := encodeDocuments(t, test.documents)
			documents, err := decodeDocuments(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(documents, test.documents) {
				t.Fatalf("documents = %#v, want %#v", documents, test.documents)
			}
			if got := hex.EncodeToString(encoded); got != test.wantHex {
				t.Fatalf("sidecar bytes = %s, want %s", got, test.wantHex)
			}
		})
	}
}

func TestDocumentWriterRejectsInvalidExternalIDs(t *testing.T) {
	tests := []string{"", string([]byte{0xff}), string(make([]byte, corpus.MaxExternalIDBytes+1))}
	for _, externalID := range tests {
		writer := newTestDocumentWriter(t)
		if err := writer.write(index.DocumentMeta{ExternalID: externalID}); err == nil {
			t.Fatal("write() error = nil")
		}
		if err := writer.close(); err == nil {
			t.Fatal("close() error = nil")
		}
	}
}

func TestDocumentReaderRejectsInvalidData(t *testing.T) {
	valid := encodeDocuments(t, []index.DocumentMeta{{ExternalID: "id", Length: 2}})
	tests := []struct {
		name    string
		corrupt func([]byte) []byte
	}{
		{name: "wrong magic", corrupt: func(data []byte) []byte {
			data[0] = 'X'
			return data
		}},
		{name: "oversized external ID", corrupt: func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[8:12], corpus.MaxExternalIDBytes+1)
			return data
		}},
		{name: "invalid UTF-8 external ID", corrupt: func(data []byte) []byte {
			data[12] = 0xff
			return data
		}},
		{name: "truncated document", corrupt: func(data []byte) []byte {
			return data[:15]
		}},
		{name: "missing end marker", corrupt: func(data []byte) []byte {
			return data[:len(data)-4]
		}},
		{name: "trailing byte", corrupt: func(data []byte) []byte {
			return append(data, 0)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(nil), valid...)
			if _, err := decodeDocuments(test.corrupt(data)); err == nil {
				t.Fatal("decodeDocuments() error = nil")
			}
		})
	}
}

func TestDocumentWriterCloseReportsOutputErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	writer, err := newDocumentWriter(&failingWriteCloser{writeErr: writeErr, closeErr: closeErr})
	if err != nil {
		t.Fatal(err)
	}

	err = writer.close()
	if !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("close() error = %v, want write and close errors", err)
	}
}

func newTestDocumentWriter(t *testing.T) *documentWriter {
	t.Helper()
	writer, err := newDocumentWriter(&bufferWriteCloser{})
	if err != nil {
		t.Fatal(err)
	}
	return writer
}

func encodeDocuments(t *testing.T, documents []index.DocumentMeta) []byte {
	t.Helper()
	output := &bufferWriteCloser{}
	writer, err := newDocumentWriter(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, document := range documents {
		if err := writer.write(document); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func decodeDocuments(data []byte) ([]index.DocumentMeta, error) {
	reader, err := newDocumentReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var documents []index.DocumentMeta
	for {
		document, err := reader.next()
		if err == io.EOF {
			return documents, nil
		}
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
}
