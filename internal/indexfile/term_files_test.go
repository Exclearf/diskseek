package indexfile

import (
	"bytes"
	"io"
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
	if err := writeTermBodies(&termBody, &postingBody, source.nextTerm, source.nextPosting); err != nil {
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
		source.nextTerm,
		source.nextPosting,
	)
	if err != nil {
		t.Fatal(err)
	}

	wantTerms := "DSKTERM\x01" +
		"\x02\x00\x00\x00\x02\x00\x00\x00\x00\x00\x00\x00" +
		"\x18\x00\x00\x00\x00\x00\x00\x00go" +
		"\x06\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00" +
		"\x10\x00\x00\x00\x00\x00\x00\x00search" +
		"\x02\xaf\x50\xfd"
	wantPostings := "DSKPOST\x01" +
		"\x01\x00\x00\x00\x10\x00\x00\x00" +
		"\x00\x00\x00\x00\x01\x00\x00\x00" +
		"\x01\x00\x00\x00\x03\x00\x00\x00" +
		"\x00\x00\x00\x00\x08\x00\x00\x00" +
		"\x00\x00\x00\x00\x01\x00\x00\x00" +
		"\xec\x63\x54\x3d"

	tests := []struct {
		name     string
		got      []byte
		want     string
		metadata FileMetadata
	}{
		{"terms", terms.Bytes(), wantTerms, metadata.Terms},
		{"postings", postings.Bytes(), wantPostings, metadata.Postings},
	}
	wantMetadata := []FileMetadata{
		{Length: 60, Checksum: 0xfd50af02},
		{Length: 52, Checksum: 0x3d5463ec},
	}
	for position, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !bytes.Equal(test.got, []byte(test.want)) {
				t.Fatalf("file = % x, want % x", test.got, test.want)
			}
			if test.metadata != wantMetadata[position] {
				t.Fatalf("metadata = %+v, want %+v", test.metadata, wantMetadata[position])
			}
		})
	}
}
