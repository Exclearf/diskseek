package search

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/segment"
)

type searchModelConfig struct {
	seed               uint64
	documentCount      int
	emptyDocumentCount int
	vocabularySize     int
	maxDocumentLength  int
	randomQueryCount   int
}

type searchModel struct {
	input   []byte
	queries []differentialQuery
}

func generateSearchModel(config searchModelConfig) searchModel {
	random := rand.New(rand.NewPCG(config.seed, 1))
	vocabulary := []string{"common", "rare", "tie"}
	for len(vocabulary) < config.vocabularySize {
		vocabulary = append(vocabulary, fmt.Sprintf("term%d", len(vocabulary)-3))
	}

	nonemptyDocuments := config.documentCount - config.emptyDocumentCount
	var input strings.Builder
	for documentID := range config.documentCount {
		fmt.Fprintf(&input, "document-%d\t", documentID)
		if documentID >= nonemptyDocuments {
			input.WriteByte('\n')
			continue
		}

		length := random.IntN(config.maxDocumentLength) + 1
		switch documentID {
		case 0, 1, 2:
			length = 2
		case 3:
			length = 1
		case 4:
			length = config.maxDocumentLength
		}
		if documentID >= 5 && documentID <= config.vocabularySize {
			length = max(length, 2)
		}

		input.WriteString("common")
		for position := 1; position < length; position++ {
			input.WriteByte(' ')
			switch {
			case documentID < 2:
				input.WriteString("tie")
			case documentID == 2:
				input.WriteString("rare")
			case documentID == 4:
				input.WriteString(vocabulary[3])
			case position == 1 && documentID <= config.vocabularySize:
				input.WriteString(vocabulary[documentID-1])
			default:
				input.WriteString(vocabulary[random.IntN(len(vocabulary)-3)+3])
			}
		}
		input.WriteByte('\n')
	}

	queries := []differentialQuery{
		{query: "common", k: 1},
		{query: "rare common", k: 5},
		{query: "tie", k: 2},
		{query: "missing", k: config.documentCount + 1},
		{query: "common common rare", k: 0},
		{query: "common rare tie", k: 5},
		{query: "tie common rare", k: 5},
		{query: "common rare tie tie", k: 5},
		{query: "common rare tie", k: 2},
		{query: "common rare tie", k: 7},
	}
	for range config.randomQueryCount {
		termCount := random.IntN(5) + 1
		terms := make([]string, termCount)
		for position := range terms {
			terms[position] = vocabulary[random.IntN(len(vocabulary))]
		}
		queries = append(queries, differentialQuery{
			query: strings.Join(terms, " "),
			k:     random.IntN(config.documentCount + 2),
		})
	}

	return searchModel{input: []byte(input.String()), queries: queries}
}

func fixedSearchModelConfig(seed uint64) searchModelConfig {
	return searchModelConfig{
		seed:               seed,
		documentCount:      131,
		emptyDocumentCount: 2,
		vocabularySize:     8,
		maxDocumentLength:  32,
		randomQueryCount:   5,
	}
}

func TestSearchModelIsReproducible(t *testing.T) {
	config := fixedSearchModelConfig(13)

	t.Run(fmt.Sprintf("seed-%d", config.seed), func(t *testing.T) {
		first := generateSearchModel(config)
		second := generateSearchModel(config)
		if !bytes.Equal(first.input, second.input) || !slices.Equal(first.queries, second.queries) {
			t.Fatalf("seed %d did not reproduce the same model", config.seed)
		}

		logical, err := index.Build(corpus.NewTSVReader(bytes.NewReader(first.input)))
		if err != nil {
			t.Fatal(err)
		}
		if len(logical.Documents) != config.documentCount ||
			logical.DocumentsWithTerms != uint64(config.documentCount-config.emptyDocumentCount) {
			t.Fatalf(
				"seed %d generated %d documents, %d nonempty; want %d, %d",
				config.seed,
				len(logical.Documents),
				logical.DocumentsWithTerms,
				config.documentCount,
				config.documentCount-config.emptyDocumentCount,
			)
		}
		if len(logical.Postings) != config.vocabularySize {
			t.Fatalf("seed %d generated vocabulary size %d, want %d", config.seed, len(logical.Postings), config.vocabularySize)
		}
		if len(logical.Postings["common"]) != config.documentCount-config.emptyDocumentCount ||
			len(logical.Postings["rare"]) != 1 || len(logical.Postings["tie"]) != 2 {
			t.Fatalf("seed %d did not generate the common, rare, and tie posting lists", config.seed)
		}
		if logical.Documents[3].Length != 1 ||
			logical.Documents[4].Length != uint32(config.maxDocumentLength) {
			t.Fatalf("seed %d did not generate the document-length extremes", config.seed)
		}
		termZero := logical.Postings["term0"]
		if len(termZero) == 0 || termZero[0].DocumentID != 4 ||
			termZero[0].Frequency != uint32(config.maxDocumentLength-1) {
			t.Fatalf("seed %d did not generate the maximum term frequency", config.seed)
		}
		tied, err := referenceSearch(&logical, "tie", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(tied) != 2 || math.Float64bits(tied[0].Score) != math.Float64bits(tied[1].Score) {
			t.Fatalf("seed %d did not generate an exact score tie", config.seed)
		}

		destination := filepath.Join(t.TempDir(), "index")
		_, err = segment.BuildIndex(
			context.Background(),
			corpus.NewTSVReader(bytes.NewReader(first.input)),
			destination,
			segment.BuildOptions{
				FlushTarget:        1,
				MergeFanIn:         2,
				MergeWorkers:       2,
				Codec:              indexfile.PostingsCodecVByte,
				TemporaryDirectory: t.TempDir(),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := indexfile.Verify(context.Background(), destination); err != nil {
			t.Fatal(err)
		}
	})
}
