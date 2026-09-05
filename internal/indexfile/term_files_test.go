package indexfile

import (
	"bytes"
	"io"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

type termTestTerm struct {
	term     string
	postings []index.Posting
}

type termTestSource struct {
	terms        []termTestTerm
	termIndex    int
	postingIndex int
}

func (s *termTestSource) nextTerm() (string, uint64, error) {
	if s.termIndex == len(s.terms) {
		return "", 0, io.EOF
	}
	current := s.terms[s.termIndex]
	s.termIndex++
	s.postingIndex = 0
	return current.term, uint64(len(current.postings)), nil
}

func (s *termTestSource) nextPosting() (index.Posting, error) {
	current := s.terms[s.termIndex-1].postings
	if s.postingIndex == len(current) {
		return index.Posting{}, io.EOF
	}
	posting := current[s.postingIndex]
	s.postingIndex++
	return posting, nil
}

func TestWriteTermBodiesRecordsWrittenPostingLengths(t *testing.T) {
	source := termTestSource{terms: []termTestTerm{
		{
			term: "go",
			postings: []index.Posting{
				{DocumentID: 0, Frequency: 1},
				{DocumentID: 1, Frequency: 3},
			},
		},
		{
			term:     "yak",
			postings: []index.Posting{{DocumentID: 0, Frequency: 1}},
		},
	}}

	var termBody, postingBody bytes.Buffer
	if err := writeTermBodies(
		&termBody,
		&postingBody,
		PostingsCodecRaw,
		source.nextTerm,
		source.nextPosting,
	); err != nil {
		t.Fatal(err)
	}

	wantTerms := append(append([]byte(nil), goTermRecord...), yakTermRecord...)
	if !bytes.Equal(termBody.Bytes(), wantTerms) {
		t.Fatalf("term body = % x, want % x", termBody.Bytes(), wantTerms)
	}
	if postingBody.Len() != 40 {
		t.Fatalf("posting body length = %d, want 40", postingBody.Len())
	}
}

func TestWriteTermBodiesSelectsVBytePostings(t *testing.T) {
	want := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 129, Frequency: 2},
	}
	source := termTestSource{terms: []termTestTerm{{term: "go", postings: want}}}

	var termBody, postingBody bytes.Buffer
	if err := writeTermBodies(
		&termBody,
		&postingBody,
		PostingsCodecVByte,
		source.nextTerm,
		source.nextPosting,
	); err != nil {
		t.Fatal(err)
	}

	record, err := readTermRecord(
		bytes.NewReader(termBody.Bytes()),
		uint64(termBody.Len()),
		2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.postingsBytes != uint64(postingBody.Len()) {
		t.Fatalf("recorded postings bytes = %d, want %d", record.postingsBytes, postingBody.Len())
	}

	var got []index.Posting
	if err := readVBytePostingList(
		bytes.NewReader(postingBody.Bytes()),
		uint64(len(want)),
		record.postingsBytes,
		130,
		func(posting index.Posting) error {
			got = append(got, posting)
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("postings = %v, want %v", got, want)
	}
}

func TestWriteTermBodiesRejectsUnsupportedCodec(t *testing.T) {
	err := writeTermBodies(
		io.Discard,
		io.Discard,
		PostingsCodec(3),
		func() (string, uint64, error) { return "", 0, io.EOF },
		func() (index.Posting, error) { return index.Posting{}, io.EOF },
	)
	if err == nil {
		t.Fatal("writeTermBodies() error = nil")
	}
}

func TestWriteTermFiles(t *testing.T) {
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

	var terms, postings bytes.Buffer
	metadata, err := WriteTermFiles(
		&terms,
		&postings,
		PostingsCodecRaw,
		source.nextTerm,
		source.nextPosting,
	)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Codec != PostingsCodecRaw {
		t.Fatalf("postings codec = %d, want %d", metadata.Codec, PostingsCodecRaw)
	}

	tests := []struct {
		name     string
		got      []byte
		want     []byte
		metadata FileMetadata
	}{
		{"terms", terms.Bytes(), readGoldenIndexFile(t, TermsFileName), metadata.Terms},
		{"postings", postings.Bytes(), readGoldenIndexFile(t, PostingsFileName), metadata.Postings},
	}
	wantMetadata := []FileMetadata{
		{Length: 60, Checksum: 0xfd50af02},
		{Length: 52, Checksum: 0x3d5463ec},
	}
	for position, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !bytes.Equal(test.got, test.want) {
				t.Fatalf("file = % x, want % x", test.got, test.want)
			}
			if test.metadata != wantMetadata[position] {
				t.Fatalf("metadata = %+v, want %+v", test.metadata, wantMetadata[position])
			}
		})
	}
}
