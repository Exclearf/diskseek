package segment

import "github.com/Exclearf/diskseek/internal/index"

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

func (s *segmentState) addDocument(tokens []string) uint64 {
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
	return segmentBufferBytes + s.retainedBytes + documentBytes
}
