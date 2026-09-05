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
		{DocumentID: 1, Frequency: 3},
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
		3,
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.postingsBytes != uint64(postingBody.Len()) {
		t.Fatalf("recorded postings bytes = %d, want %d", record.postingsBytes, postingBody.Len())
	}
	if record.maxTermFrequency != 3 {
		t.Fatalf("maximum term frequency = %d, want 3", record.maxTermFrequency)
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
	tests := []struct {
		name      string
		directory string
		codec     PostingsCodec
		metadata  TermFilesMetadata
	}{
		{"raw", goldenIndexDirectory, PostingsCodecRaw, goldenTermMetadata},
		{"vbyte", goldenVByteIndexDirectory, PostingsCodecVByte, goldenVByteTermMetadata},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := termTestSource{terms: verificationTestTerms()}
			var terms, postings bytes.Buffer
			metadata, err := WriteTermFiles(
				&terms,
				&postings,
				test.codec,
				source.nextTerm,
				source.nextPosting,
			)
			if err != nil {
				t.Fatal(err)
			}
			if metadata != test.metadata {
				t.Fatalf("metadata = %+v, want %+v", metadata, test.metadata)
			}

			wantTerms := readGoldenIndexFileFrom(t, test.directory, TermsFileName)
			if !bytes.Equal(terms.Bytes(), wantTerms) {
				t.Fatalf("terms file = % x, want % x", terms.Bytes(), wantTerms)
			}
			wantPostings := readGoldenIndexFileFrom(t, test.directory, PostingsFileName)
			if !bytes.Equal(postings.Bytes(), wantPostings) {
				t.Fatalf("postings file = % x, want % x", postings.Bytes(), wantPostings)
			}
		})
	}
}

func TestPostingCodecSizeEvidence(t *testing.T) {
	tests := []struct {
		name  string
		count int
		raw   codecSize
		vbyte codecSize
	}{
		{
			name:  "short",
			count: 8,
			raw: codecSize{
				payloadBytes: 64, blockHeaderBytes: 8, postingsFileBytes: 84,
				termBytes: 40, documentBytes: 148, metadataBytes: 80, wholeIndexBytes: 352,
			},
			vbyte: codecSize{
				payloadBytes: 16, blockHeaderBytes: 8, postingsFileBytes: 36,
				termBytes: 40, documentBytes: 148, metadataBytes: 80, wholeIndexBytes: 304,
			},
		},
		{
			name:  "medium",
			count: 128,
			raw: codecSize{
				payloadBytes: 1024, blockHeaderBytes: 8, postingsFileBytes: 1044,
				termBytes: 40, documentBytes: 1708, metadataBytes: 80, wholeIndexBytes: 2872,
			},
			vbyte: codecSize{
				payloadBytes: 256, blockHeaderBytes: 8, postingsFileBytes: 276,
				termBytes: 40, documentBytes: 1708, metadataBytes: 80, wholeIndexBytes: 2104,
			},
		},
		{
			name:  "long",
			count: 4096,
			raw: codecSize{
				payloadBytes: 32768, blockHeaderBytes: 256, postingsFileBytes: 33036,
				termBytes: 40, documentBytes: 53292, metadataBytes: 80, wholeIndexBytes: 86448,
			},
			vbyte: codecSize{
				payloadBytes: 8223, blockHeaderBytes: 256, postingsFileBytes: 8491,
				termBytes: 40, documentBytes: 53292, metadataBytes: 80, wholeIndexBytes: 61903,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, codec := range []struct {
				name string
				id   PostingsCodec
				want codecSize
			}{
				{name: "raw", id: PostingsCodecRaw, want: test.raw},
				{name: "vbyte", id: PostingsCodecVByte, want: test.vbyte},
			} {
				t.Run(codec.name, func(t *testing.T) {
					got := measureCodecSize(t, test.count, codec.id)
					if got != codec.want {
						t.Fatalf("size = %+v, want %+v", got, codec.want)
					}
					postingBytes := got.payloadBytes + got.blockHeaderBytes
					t.Logf(
						"postings=%d payload=%d block_headers=%d posting_bytes=%d bytes/posting=%.6f postings_file=%d terms_file=%d document_files=%d metadata_file=%d whole_index=%d",
						test.count,
						got.payloadBytes,
						got.blockHeaderBytes,
						postingBytes,
						float64(postingBytes)/float64(test.count),
						got.postingsFileBytes,
						got.termBytes,
						got.documentBytes,
						got.metadataBytes,
						got.wholeIndexBytes,
					)
				})
			}
		})
	}
}

type codecSize struct {
	payloadBytes      uint64
	blockHeaderBytes  uint64
	postingsFileBytes uint64
	termBytes         uint64
	documentBytes     uint64
	metadataBytes     uint64
	wholeIndexBytes   uint64
}

func measureCodecSize(t *testing.T, documentCount int, codec PostingsCodec) codecSize {
	t.Helper()

	postings := make([]index.Posting, documentCount)
	for documentID := range postings {
		postings[documentID] = index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
	}
	source := termTestSource{terms: []termTestTerm{{term: "term", postings: postings}}}
	var terms, postingData bytes.Buffer
	termMetadata, err := WriteTermFiles(&terms, &postingData, codec, source.nextTerm, source.nextPosting)
	if err != nil {
		t.Fatal(err)
	}

	documentIndex := 0
	var lengths, offsets, externalIDs bytes.Buffer
	documentMetadata, err := WriteDocumentFiles(&lengths, &offsets, &externalIDs, func() (index.DocumentMeta, error) {
		if documentIndex == documentCount {
			return index.DocumentMeta{}, io.EOF
		}
		documentIndex++
		return index.DocumentMeta{ExternalID: "d", Length: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var metadata bytes.Buffer
	if err := WriteMetadataFile(&metadata, termMetadata, documentMetadata); err != nil {
		t.Fatal(err)
	}

	blockCount := (uint64(documentCount)-1)/uint64(postingsPerBlock) + 1
	blockHeaderBytes := blockCount * uint64(postingBlockHeaderBytes)
	postingBodyBytes := uint64(postingData.Len()) - minimumFileBytes
	documentBytes := uint64(lengths.Len() + offsets.Len() + externalIDs.Len())
	return codecSize{
		payloadBytes:      postingBodyBytes - blockHeaderBytes,
		blockHeaderBytes:  blockHeaderBytes,
		postingsFileBytes: uint64(postingData.Len()),
		termBytes:         uint64(terms.Len()),
		documentBytes:     documentBytes,
		metadataBytes:     uint64(metadata.Len()),
		wholeIndexBytes:   uint64(metadata.Len()+terms.Len()+postingData.Len()) + documentBytes,
	}
}
