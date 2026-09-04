package segment

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

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

func TestMergeRunGroupStreamsHotTerm(t *testing.T) {
	const fanIn = 3
	const accountedBufferBytes = (fanIn + 1) * runBufferBytes

	for _, documentsPerRun := range []uint64{1 << 12, 1 << 16} {
		flow := &mergeFlow{}
		inputs := make([]io.Reader, fanIn)
		for runOrdinal := range fanIn {
			firstDocumentID := index.DocumentID(uint64(runOrdinal) * documentsPerRun)
			input := bytes.NewReader(encodeHotTermRun(t, firstDocumentID, documentsPerRun))
			inputs[runOrdinal] = &mergeFlowReader{Reader: input, flow: flow}
		}

		if err := mergeRunGroup(inputs, flow); err != nil {
			t.Fatal(err)
		}

		totalPostings := uint64(fanIn) * documentsPerRun
		wantOutputBytes := int64(runHeaderBytes+4+len("hot")+8+4) + int64(totalPostings)*8
		if flow.writtenBytes != wantOutputBytes {
			t.Fatalf("%d postings: output bytes = %d, want %d", totalPostings, flow.writtenBytes, wantOutputBytes)
		}
		if flow.maxReadAheadBytes > accountedBufferBytes {
			t.Fatalf("%d postings: maximum read-ahead = %d bytes, want at most %d", totalPostings, flow.maxReadAheadBytes, accountedBufferBytes)
		}
		t.Logf("postings=%d, fixed buffers=%d bytes, maximum read-ahead=%d bytes", totalPostings, accountedBufferBytes, flow.maxReadAheadBytes)
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

func TestMergeRunGroupReportsInputErrors(t *testing.T) {
	readErr := errors.New("read input")
	closeErr := errors.New("close output")
	hotRun := encodeHotTermRun(t, 0, 2)
	secondRun := encodeMergeTestRun(t, runHeader{firstDocumentID: 2, documentCount: 1}, nil)
	const firstPostingEnd = runHeaderBytes + 4 + len("hot") + 8 + 8
	tests := []struct {
		name  string
		input io.Reader
	}{
		{name: "run header", input: iotest.ErrReader(readErr)},
		{
			name: "hot term postings",
			input: io.MultiReader(
				io.LimitReader(bytes.NewReader(hotRun), int64(firstPostingEnd)),
				iotest.ErrReader(readErr),
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := &mergeCloseErrorBuffer{closeErr: closeErr}
			err := mergeRunGroup([]io.Reader{test.input, bytes.NewReader(secondRun)}, output)
			if !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
				t.Fatalf("mergeRunGroup() error = %v, want read and close errors", err)
			}
		})
	}
}

func TestMergeRunGroupReportsOutputErrors(t *testing.T) {
	inputs := []io.Reader{
		bytes.NewReader(encodeMergeTestRun(t, runHeader{documentCount: 1}, nil)),
		bytes.NewReader(encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, nil)),
	}
	writeErr := errors.New("write output")
	closeErr := errors.New("close output")
	output := &failingWriteCloser{writeErr: writeErr, closeErr: closeErr}
	if err := mergeRunGroup(inputs, output); !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("mergeRunGroup() error = %v, want write and close errors", err)
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

func encodeHotTermRun(t *testing.T, firstDocumentID index.DocumentID, documentCount uint64) []byte {
	t.Helper()
	output := &bufferWriteCloser{}
	writer, err := newRunWriter(output, runHeader{
		firstDocumentID: firstDocumentID,
		documentCount:   documentCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.writeTerm("hot", documentCount); err != nil {
		t.Fatal(err)
	}
	for offset := range documentCount {
		posting := index.Posting{
			DocumentID: firstDocumentID + index.DocumentID(offset),
			Frequency:  1,
		}
		if err := writer.writePosting(posting); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

type mergeFlow struct {
	readBytes         int64
	writtenBytes      int64
	maxReadAheadBytes int64
}

func (f *mergeFlow) Write(buffer []byte) (int, error) {
	f.writtenBytes += int64(len(buffer))
	return len(buffer), nil
}

func (*mergeFlow) Close() error {
	return nil
}

type mergeFlowReader struct {
	io.Reader
	flow *mergeFlow
}

func (r *mergeFlowReader) Read(buffer []byte) (int, error) {
	read, err := r.Reader.Read(buffer)
	r.flow.readBytes += int64(read)
	r.flow.maxReadAheadBytes = max(r.flow.maxReadAheadBytes, r.flow.readBytes-r.flow.writtenBytes)
	return read, err
}

type mergeCloseErrorBuffer struct {
	bytes.Buffer
	closeErr error
}

func (b *mergeCloseErrorBuffer) Close() error {
	return b.closeErr
}
