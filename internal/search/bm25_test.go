package search

import (
	"bytes"
	"math"
	"math/rand"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
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

func TestBM25TermScoreMaterialization(t *testing.T) {
	// These values differ by one bit if the second contribution is fused with the sum.
	const idf = 0.1
	score := bm25TermScore(idf, 1, 4, 4)
	score += bm25TermScore(idf, 4, 4, 4)

	const wantBits = 0x3fd05397829cbc14
	if got := math.Float64bits(score); got != wantBits {
		t.Fatalf("score bits = %#x, want %#x", got, uint64(wantBits))
	}
}

func TestBM25TermUpperBound(t *testing.T) {
	const (
		idf                   = 0.5
		maxTermFrequency      = 3
		averageDocumentLength = 2.5
	)

	got := bm25TermUpperBound(idf, maxTermFrequency, averageDocumentLength)
	want := bm25TermScore(idf, maxTermFrequency, maxTermFrequency, averageDocumentLength)
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("upper bound = %v, want %v", got, want)
	}
}

func TestBM25TermUpperBoundDominatesValidPostings(t *testing.T) {
	const generatedCases = 10_000

	check := func(
		documentsWithTerms, documentFrequency uint64,
		maxTermFrequency, termFrequency, documentLength uint32,
		averageDocumentLength float64,
	) {
		t.Helper()
		idf := bm25IDF(documentsWithTerms, documentFrequency)
		score := bm25TermScore(idf, termFrequency, documentLength, averageDocumentLength)
		bound := bm25TermUpperBound(idf, maxTermFrequency, averageDocumentLength)
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 ||
			math.IsNaN(bound) || math.IsInf(bound, 0) || bound < 0 || score > bound {
			t.Fatalf(
				"invalid score %v or bound %v for tf=%d, length=%d, maxTF=%d, avgdl=%v, N=%d, df=%d",
				score,
				bound,
				termFrequency,
				documentLength,
				maxTermFrequency,
				averageDocumentLength,
				documentsWithTerms,
				documentFrequency,
			)
		}
	}

	check(1, 1, 1, 1, 1, 1)
	check(1, 1, math.MaxUint32, 1, math.MaxUint32, 1)
	check(1_000_000, 1, math.MaxUint32, math.MaxUint32, math.MaxUint32, math.MaxUint32)
	check(math.MaxUint32, math.MaxUint32, math.MaxUint32, math.MaxUint32-1, math.MaxUint32-1, math.MaxUint32)

	random := rand.New(rand.NewSource(0))
	for range generatedCases {
		documentsWithTerms := random.Uint64()%1_000_000 + 1
		documentFrequency := random.Uint64()%documentsWithTerms + 1

		maxTermFrequency := random.Uint32()
		if maxTermFrequency == 0 {
			maxTermFrequency = 1
		}
		termFrequency := uint32(random.Uint64()%uint64(maxTermFrequency)) + 1
		documentLength := termFrequency + uint32(
			random.Uint64()%(uint64(math.MaxUint32)-uint64(termFrequency)+1),
		)
		averageDocumentLength := 1 + random.Float64()*(float64(math.MaxUint32)-1)

		check(
			documentsWithTerms,
			documentFrequency,
			maxTermFrequency,
			termFrequency,
			documentLength,
			averageDocumentLength,
		)
	}
}

func TestDecodedPostingScoresRespectMaxTFBounds(t *testing.T) {
	model := generateSearchModel(fixedSearchModelConfig(13))
	logical, err := index.Build(corpus.NewTSVReader(bytes.NewReader(model.input)))
	if err != nil {
		t.Fatal(err)
	}
	destination := buildDifferentialIndex(t, model.input, indexfile.PostingsCodecVByte, 1)
	disk := openDiskTestIndex(t, destination)

	for term := range logical.Postings {
		cursor, found, err := disk.Postings(term)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("term %q is missing", term)
		}
		idf := bm25IDF(disk.DocumentsWithTerms(), cursor.DocumentFrequency())
		bound := bm25TermUpperBound(idf, cursor.MaxTermFrequency(), disk.AverageDocumentLength())
		if math.IsNaN(bound) || math.IsInf(bound, 0) || bound < 0 {
			t.Fatalf("term %q has invalid bound %v", term, bound)
		}

		for {
			posting, valid := cursor.Current()
			if !valid {
				break
			}
			score := bm25TermScore(
				idf,
				posting.Frequency,
				disk.DocumentLength(posting.DocumentID),
				disk.AverageDocumentLength(),
			)
			if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > bound {
				t.Fatalf("term %q document %d has score %v with bound %v", term, posting.DocumentID, score, bound)
			}
			if _, err := cursor.Next(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
