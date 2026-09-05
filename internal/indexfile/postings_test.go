package indexfile

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestVerifyPostingsFile(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		data := writeIndexFileTestData(t, postingsRole, nil)
		if err := verifyPostingsFile(
			context.Background(),
			bytes.NewReader(data),
			int64(len(data)),
			nil,
			nil,
		); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("postings", func(t *testing.T) {
		data, terms := postingVerificationFixture(t)
		if err := verifyPostingsFile(
			context.Background(),
			bytes.NewReader(data),
			int64(len(data)),
			terms,
			[]uint32{2, 3},
		); err != nil {
			t.Fatal(err)
		}
	})
}

func TestVerifyPostingsFileRejectsInconsistentDocumentLengths(t *testing.T) {
	tests := []struct {
		name    string
		lengths []uint32
	}{
		{name: "frequencies exceed length", lengths: []uint32{1, 4}},
		{name: "frequencies do not reach length", lengths: []uint32{3, 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, terms := postingVerificationFixture(t)
			if err := verifyPostingsFile(
				context.Background(),
				bytes.NewReader(data),
				int64(len(data)),
				terms,
				test.lengths,
			); err == nil {
				t.Fatal("verifyPostingsFile() error = nil")
			}
		})
	}
}

func TestVerifyPostingsFileValidatesChecksum(t *testing.T) {
	data, terms := postingVerificationFixture(t)
	data[fileHeaderBytes+rawPostingBlockHeaderBytes+4] = 2
	if err := verifyPostingsFile(
		context.Background(),
		bytes.NewReader(data),
		int64(len(data)),
		terms,
		[]uint32{3, 3},
	); err == nil {
		t.Fatal("verifyPostingsFile() error = nil")
	}
}

func TestVerifyPostingsFileCancellation(t *testing.T) {
	data, terms := postingVerificationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := &cancelAfterReader{
		Reader: bytes.NewReader(data),
		after:  fileHeaderBytes + rawPostingBlockHeaderBytes,
		cancel: cancel,
	}
	err := verifyPostingsFile(ctx, input, int64(len(data)), terms, []uint32{2, 3})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("verifyPostingsFile() error = %v, want context.Canceled", err)
	}
	if input.readBytes >= len(data) {
		t.Fatal("verification read the complete postings file after cancellation")
	}
}

type cancelAfterReader struct {
	Reader    *bytes.Reader
	after     int
	readBytes int
	cancel    context.CancelFunc
}

func (r *cancelAfterReader) Read(data []byte) (int, error) {
	read, err := r.Reader.Read(data)
	r.readBytes += read
	if r.cancel != nil && r.readBytes >= r.after {
		r.cancel()
		r.cancel = nil
	}
	return read, err
}

func postingVerificationFixture(t *testing.T) ([]byte, map[string]termEntry) {
	t.Helper()
	source := termTestSource{terms: []termTestTerm{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
		},
		{
			term:     "search",
			postings: []index.Posting{{DocumentID: 0, Frequency: 1}},
		},
	}}
	var termData, postingData bytes.Buffer
	if _, err := WriteTermFiles(&termData, &postingData, source.nextTerm, source.nextPosting); err != nil {
		t.Fatal(err)
	}
	return postingData.Bytes(), map[string]termEntry{
		"go":     {documentFrequency: 2, postingsOffset: 8, postingsBytes: 24},
		"search": {documentFrequency: 1, postingsOffset: 32, postingsBytes: 16},
	}
}
