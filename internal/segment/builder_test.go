package segment

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

func TestBuildRunsAtDocumentBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		target      uint64
		wantHeaders []runHeader
	}{
		{name: "empty corpus", target: segmentBufferBytes},
		{
			name:   "flush when target is reached",
			input:  "0\ta a\n1\ta b\n",
			target: segmentBufferBytes + 14,
			wantHeaders: []runHeader{
				{firstDocumentID: 0, documentCount: 1},
				{firstDocumentID: 1, documentCount: 1},
			},
		},
		{
			name:        "flush remaining documents at EOF",
			input:       "0\ta a\n1\ta b\n",
			target:      segmentBufferBytes + 40,
			wantHeaders: []runHeader{{firstDocumentID: 0, documentCount: 2}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var outputs []*bufferWriteCloser
			documentOutput := &bufferWriteCloser{}
			_, err := buildRuns(
				corpus.NewTSVReader(strings.NewReader(test.input)),
				test.target,
				documentOutput,
				func() (io.WriteCloser, error) {
					output := &bufferWriteCloser{}
					outputs = append(outputs, output)
					return output, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}

			var headers []runHeader
			for _, output := range outputs {
				reader, err := newRunReader(bytes.NewReader(output.Bytes()))
				if err != nil {
					t.Fatal(err)
				}
				headers = append(headers, reader.header)
			}
			if !reflect.DeepEqual(headers, test.wantHeaders) {
				t.Fatalf("run headers = %+v, want %+v", headers, test.wantHeaders)
			}
		})
	}
}

func TestBuildRunsWritesDocumentMetadataAndStatistics(t *testing.T) {
	documentOutput := &bufferWriteCloser{}
	stats, err := buildRuns(
		corpus.NewTSVReader(strings.NewReader("shared\ta a\nshared\t---\nlast\tb\n")),
		segmentBufferBytes+1024,
		documentOutput,
		func() (io.WriteCloser, error) {
			return &bufferWriteCloser{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	documents, err := decodeDocuments(documentOutput.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	want := []index.DocumentMeta{
		{ExternalID: "shared", Length: 2},
		{ExternalID: "shared", Length: 0},
		{ExternalID: "last", Length: 1},
	}
	if !reflect.DeepEqual(documents, want) {
		t.Fatalf("documents = %#v, want %#v", documents, want)
	}

	wantStats := buildStats{
		documentCount:      3,
		documentsWithTerms: 2,
		totalTokenCount:    3,
	}
	if stats != wantStats {
		t.Fatalf("statistics = %+v, want %+v", stats, wantStats)
	}
}

func TestBuildRunsRejectsZeroFlushTarget(t *testing.T) {
	_, err := buildRuns(
		corpus.NewTSVReader(strings.NewReader("")),
		0,
		&bufferWriteCloser{},
		func() (io.WriteCloser, error) { return nil, nil },
	)
	if err == nil {
		t.Fatal("buildRuns() error = nil")
	}
}
