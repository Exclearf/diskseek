package index

import (
	"io"

	"github.com/Exclearf/diskseek/internal/analyzer"
	"github.com/Exclearf/diskseek/internal/corpus"
)

type DocumentID uint32

type DocumentMeta struct {
	ExternalID string
	Length     uint32
}

type Posting struct {
	DocumentID DocumentID
	Frequency  uint32
}

type Index struct {
	Documents   []DocumentMeta
	Postings    map[string][]Posting
	TotalLength uint64
}

func Build(records *corpus.TSVReader) (Index, error) {
	result := Index{Postings: make(map[string][]Posting)}

	for {
		record, err := records.Next()
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return Index{}, err
		}

		tokens, err := analyzer.Analyze(record.Text)
		if err != nil {
			return Index{}, err
		}

		documentID := DocumentID(len(result.Documents))
		length := uint32(len(tokens))
		result.Documents = append(result.Documents, DocumentMeta{
			ExternalID: record.ExternalID,
			Length:     length,
		})
		result.TotalLength += uint64(length)

		frequencies := make(map[string]uint32)
		for _, token := range tokens {
			frequencies[token]++
		}
		for term, frequency := range frequencies {
			result.Postings[term] = append(result.Postings[term], Posting{
				DocumentID: documentID,
				Frequency:  frequency,
			})
		}
	}
}
