package segment

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestMergeRunGroup(t *testing.T) {
	inputs := []io.Reader{
		bytes.NewReader(encodeMergeTestRun(t, runHeader{documentCount: 2}, []mergeTestTerm{
			{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}}},
			{term: "hot", postings: []index.Posting{{DocumentID: 0, Frequency: 2}, {DocumentID: 1, Frequency: 1}}},
			{term: "zebra", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
		})),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 2, documentCount: 2}, []mergeTestTerm{
			{term: "banana", postings: []index.Posting{{DocumentID: 2, Frequency: 1}}},
			{term: "hot", postings: []index.Posting{{DocumentID: 2, Frequency: 1}, {DocumentID: 3, Frequency: 3}}},
		})),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 4, documentCount: 2}, []mergeTestTerm{
			{term: "apple", postings: []index.Posting{{DocumentID: 4, Frequency: 1}}},
			{term: "hot", postings: []index.Posting{{DocumentID: 4, Frequency: 1}, {DocumentID: 5, Frequency: 2}}},
			{term: "yak", postings: []index.Posting{{DocumentID: 5, Frequency: 1}}},
		})),
	}

	output := &bufferWriteCloser{}
	if err := mergeRunGroup(inputs, output); err != nil {
		t.Fatal(err)
	}

	want := encodeMergeTestRun(t, runHeader{documentCount: 6}, []mergeTestTerm{
		{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}, {DocumentID: 4, Frequency: 1}}},
		{term: "banana", postings: []index.Posting{{DocumentID: 2, Frequency: 1}}},
		{term: "hot", postings: []index.Posting{
			{DocumentID: 0, Frequency: 2},
			{DocumentID: 1, Frequency: 1},
			{DocumentID: 2, Frequency: 1},
			{DocumentID: 3, Frequency: 3},
			{DocumentID: 4, Frequency: 1},
			{DocumentID: 5, Frequency: 2},
		}},
		{term: "yak", postings: []index.Posting{{DocumentID: 5, Frequency: 1}}},
		{term: "zebra", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
	})
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatal("merged run does not match the hand-calculated output")
	}
}

func TestMergeRunGroupRejectsInvalidGroup(t *testing.T) {
	tests := []struct {
		name    string
		headers []runHeader
	}{
		{name: "gap", headers: []runHeader{{documentCount: 1}, {firstDocumentID: 2, documentCount: 1}}},
		{name: "overlap", headers: []runHeader{{documentCount: 2}, {firstDocumentID: 1, documentCount: 1}}},
		{name: "reversed", headers: []runHeader{{firstDocumentID: 1, documentCount: 1}, {documentCount: 1}}},
		{name: "zero-document input", headers: []runHeader{{}, {documentCount: 1}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := make([]io.Reader, len(test.headers))
			for i, header := range test.headers {
				inputs[i] = bytes.NewReader(encodeMergeTestRun(t, header, nil))
			}
			if err := mergeRunGroup(inputs, &bufferWriteCloser{}); err == nil {
				t.Fatal("mergeRunGroup() error = nil")
			}
		})
	}
}

func TestMergeRunGroupPreservesNonzeroDocumentStart(t *testing.T) {
	inputs := []io.Reader{
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 7, documentCount: 1}, nil)),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 8, documentCount: 1}, nil)),
	}
	output := &bufferWriteCloser{}
	if err := mergeRunGroup(inputs, output); err != nil {
		t.Fatal(err)
	}

	want := encodeMergeTestRun(t, runHeader{firstDocumentID: 7, documentCount: 2}, nil)
	if !bytes.Equal(output.Bytes(), want) {
		t.Fatal("merged run does not preserve its document interval")
	}
}

func TestMergeRunGroupReadsInputsToCleanEnd(t *testing.T) {
	corrupt := encodeMergeTestRun(t, runHeader{documentCount: 1}, []mergeTestTerm{
		{term: "a", postings: []index.Posting{{DocumentID: 0, Frequency: 1}}},
	})
	corrupt = append(corrupt, 1)
	valid := encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, []mergeTestTerm{
		{term: "b", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
	})

	inputs := []io.Reader{bytes.NewReader(corrupt), bytes.NewReader(valid)}
	if err := mergeRunGroup(inputs, &bufferWriteCloser{}); err == nil {
		t.Fatal("mergeRunGroup() error = nil")
	}
}

func TestMergeRunGroupReportsOutputCloseError(t *testing.T) {
	inputs := []io.Reader{
		bytes.NewReader(encodeMergeTestRun(t, runHeader{documentCount: 1}, nil)),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, nil)),
	}
	closeErr := errors.New("close output")
	output := &mergeCloseErrorBuffer{closeErr: closeErr}
	if err := mergeRunGroup(inputs, output); !errors.Is(err, closeErr) {
		t.Fatalf("mergeRunGroup() error = %v, want %v", err, closeErr)
	}
}

type mergeTestTerm struct {
	term     string
	postings []index.Posting
}

func encodeMergeTestRun(t *testing.T, header runHeader, terms []mergeTestTerm) []byte {
	t.Helper()
	output := &bufferWriteCloser{}
	writer, err := newRunWriter(output, header)
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range terms {
		if err := writer.writeTerm(term.term, uint64(len(term.postings))); err != nil {
			t.Fatal(err)
		}
		for _, posting := range term.postings {
			if err := writer.writePosting(posting); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

type mergeCloseErrorBuffer struct {
	bytes.Buffer
	closeErr error
}

func (b *mergeCloseErrorBuffer) Close() error {
	return b.closeErr
}
