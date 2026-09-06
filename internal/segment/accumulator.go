package segment

import (
	"io"
	"slices"

	"github.com/Exclearf/diskseek/internal/index"
)

const segmentBufferBytes uint64 = 2 * runBufferBytes

type segmentState struct {
	firstDocumentID index.DocumentID
	documentCount   uint64
	postings        map[string][]index.Posting
	retainedBytes   uint64
}

func newSegmentState(firstDocumentID index.DocumentID) segmentState {
	return segmentState{
		firstDocumentID: firstDocumentID,
		postings:        make(map[string][]index.Posting),
	}
}

func (s *segmentState) addDocument(tokens []string) (uint64, uint64) {
	frequencies := make(map[string]uint32)
	var documentBytes uint64
	for _, token := range tokens {
		if frequencies[token] == 0 {
			documentBytes += uint64(len(token)) + 4
		}
		frequencies[token]++
	}

	newRetainedBytes := uint64(len(frequencies)) * 8
	for term := range frequencies {
		if _, exists := s.postings[term]; !exists {
			newRetainedBytes += uint64(len(term))
		}
	}

	documentID := index.DocumentID(uint64(s.firstDocumentID) + s.documentCount)
	for term, frequency := range frequencies {
		s.postings[term] = append(s.postings[term], index.Posting{
			DocumentID: documentID,
			Frequency:  frequency,
		})
	}
	s.documentCount++
	s.retainedBytes += newRetainedBytes
	return segmentBufferBytes + s.retainedBytes + documentBytes, uint64(len(frequencies))
}

func (s *segmentState) writeRun(output io.WriteCloser) error {
	writer, err := newRunWriter(output, runHeader{
		firstDocumentID: s.firstDocumentID,
		documentCount:   s.documentCount,
	})
	if err != nil {
		return err
	}

	terms := make([]string, 0, len(s.postings))
	for term := range s.postings {
		terms = append(terms, term)
	}
	slices.Sort(terms)

	for _, term := range terms {
		postings := s.postings[term]
		if err := writer.writeTerm(term, uint64(len(postings))); err != nil {
			return writer.close()
		}
		for _, posting := range postings {
			if err := writer.writePosting(posting); err != nil {
				return writer.close()
			}
		}
	}
	return writer.close()
}
