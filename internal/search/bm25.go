package search

import "math"

const (
	bm25K1 = 0.9
	bm25B  = 0.4
)

func bm25IDF(documentsWithTerms, documentFrequency uint64) float64 {
	numerator := float64(documentsWithTerms-documentFrequency) + 0.5
	denominator := float64(documentFrequency) + 0.5
	return math.Log1p(numerator / denominator)
}

func bm25TermScore(
	idf float64,
	termFrequency, documentLength uint32,
	averageDocumentLength float64,
) float64 {
	lengthRatio := float64(documentLength) / averageDocumentLength
	lengthNormalization := (1 - bm25B) + bm25B*lengthRatio
	numerator := float64(termFrequency) * (bm25K1 + 1)
	denominator := float64(termFrequency) + bm25K1*lengthNormalization
	return float64(idf * (numerator / denominator))
}
