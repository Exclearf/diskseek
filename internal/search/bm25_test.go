package search

import (
	"math"
	"testing"
)

func TestBM25TermScore(t *testing.T) {
	const (
		documentsWithTerms    = 4
		averageDocumentLength = 3
		tolerance             = 1e-12
	)

	tests := []struct {
		name              string
		documentFrequency uint64
		termFrequency     uint32
		documentLength    uint32
		want              float64
	}{
		{name: "average document", documentFrequency: 1, termFrequency: 1, documentLength: 3, want: 1.203972804325936},
		{name: "short document", documentFrequency: 1, termFrequency: 1, documentLength: 2, want: 1.285139510235550},
		{name: "long document with repeated term", documentFrequency: 1, termFrequency: 4, documentLength: 4, want: 1.822747671887871},
		{name: "term in every document", documentFrequency: 4, termFrequency: 1, documentLength: 3, want: 0.105360515657826},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			idf := bm25IDF(documentsWithTerms, test.documentFrequency)
			got := bm25TermScore(idf, test.termFrequency, test.documentLength, averageDocumentLength)
			if math.IsNaN(got) || math.Abs(got-test.want) > tolerance {
				t.Fatalf("bm25TermScore() = %.15f, want %.15f", got, test.want)
			}
		})
	}
}
