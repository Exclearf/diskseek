package segment

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
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
			err := buildRuns(
				corpus.NewTSVReader(strings.NewReader(test.input)),
				test.target,
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

func TestBuildRunsRejectsZeroFlushTarget(t *testing.T) {
	err := buildRuns(corpus.NewTSVReader(strings.NewReader("")), 0, func() (io.WriteCloser, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatal("buildRuns() error = nil")
	}
}
