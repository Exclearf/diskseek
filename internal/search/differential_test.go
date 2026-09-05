package search

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/segment"
)

type differentialQuery struct {
	query string
	k     int
}

func TestDAATMatchesReference(t *testing.T) {
	authored, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	checkDAATParity(t, authored, []differentialQuery{
		{query: "fast", k: 10},
		{query: "search index", k: 1},
		{query: "search index", k: 2},
		{query: "fast disk", k: 10},
		{query: "index disk strasse", k: 10},
		{query: "café strasse", k: 10},
		{query: "missing", k: 10},
		{query: "---", k: 10},
		{query: "fast", k: 0},
	})
}

func TestGeneratedDAATMatchesReference(t *testing.T) {
	generator := rand.New(rand.NewPCG(10, 5))
	for corpusIndex := range 6 {
		documentCount := generator.IntN(7) + 2
		input := generateDifferentialCorpus(generator, documentCount)
		queries := []differentialQuery{
			{query: "alpha", k: 1},
			{query: "missing alpha alpha", k: documentCount + 2},
			{query: "---", k: 10},
		}
		for range 6 {
			queries = append(queries, differentialQuery{
				query: generateDifferentialQuery(generator),
				k:     generator.IntN(documentCount + 3),
			})
		}

		t.Run(fmt.Sprintf("corpus-%d", corpusIndex), func(t *testing.T) {
			checkDAATParity(t, input, queries)
		})
	}
}

func checkDAATParity(t *testing.T, input []byte, queries []differentialQuery) {
	t.Helper()
	logical, err := index.Build(corpus.NewTSVReader(bytes.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}

	requestedBytes := make(map[indexfile.PostingsCodec]uint64, 2)
	for _, codec := range []struct {
		name  string
		value indexfile.PostingsCodec
	}{
		{name: "raw", value: indexfile.PostingsCodecRaw},
		{name: "vbyte", value: indexfile.PostingsCodecVByte},
	} {
		t.Run(codec.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "index")
			err := segment.BuildIndex(
				context.Background(),
				corpus.NewTSVReader(bytes.NewReader(input)),
				destination,
				segment.BuildOptions{
					FlushTarget:        1,
					MergeFanIn:         2,
					MergeWorkers:       2,
					Codec:              codec.value,
					TemporaryDirectory: t.TempDir(),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			disk := openDiskTestIndex(t, destination)

			for queryIndex, query := range queries {
				t.Run(fmt.Sprintf("query-%d", queryIndex), func(t *testing.T) {
					want, err := referenceSearch(&logical, query.query, query.k)
					if err != nil {
						t.Fatal(err)
					}
					got, stats, err := searchDAAT(disk, query.query, query.k)
					if err != nil {
						t.Fatal(err)
					}
					if !equalResultBits(got, want) {
						t.Fatalf("searchDAAT(%q, %d) = %+v, want %+v", query.query, query.k, got, want)
					}

					postings, candidates, err := expectedDAATWork(logical.Postings, query.query, query.k)
					if err != nil {
						t.Fatal(err)
					}
					if stats.PostingsDecoded != postings || stats.NextCalls != postings ||
						stats.CandidatesScored != candidates || stats.AdvanceCalls != 0 {
						t.Fatalf("stats = %+v, want postings/next %d, candidates %d, advances 0", stats, postings, candidates)
					}
					requestedBytes[codec.value] += stats.BytesRequested
				})
			}
		})
	}
	if requestedBytes[indexfile.PostingsCodecRaw] == requestedBytes[indexfile.PostingsCodecVByte] {
		t.Fatalf("raw and VByte requested the same %d posting bytes", requestedBytes[indexfile.PostingsCodecRaw])
	}
}

func expectedDAATWork(
	postings map[string][]index.Posting,
	query string,
	k int,
) (uint64, uint64, error) {
	if k <= 0 {
		return 0, 0, nil
	}
	terms, err := prepareQuery(query)
	if err != nil {
		return 0, 0, err
	}

	documents := make(map[index.DocumentID]struct{})
	var postingCount uint64
	for _, term := range terms {
		postingCount += uint64(len(postings[term]))
		for _, posting := range postings[term] {
			documents[posting.DocumentID] = struct{}{}
		}
	}
	return postingCount, uint64(len(documents)), nil
}

func generateDifferentialCorpus(generator *rand.Rand, documentCount int) []byte {
	terms := [...]string{"alpha", "beta", "gamma", "delta", "epsilon"}
	var input strings.Builder
	for documentID := range documentCount {
		fmt.Fprintf(&input, "document-%d\t", documentID)
		length := generator.IntN(9)
		switch documentID {
		case 0:
			length = 1
		case 1:
			length = 32
		}
		for position := range length {
			if position != 0 {
				input.WriteByte(' ')
			}
			term := terms[generator.IntN(len(terms))]
			if position == 0 && (documentID == 0 || documentID == 1) {
				term = "alpha"
			}
			input.WriteString(term)
		}
		input.WriteByte('\n')
	}
	return []byte(input.String())
}

func generateDifferentialQuery(generator *rand.Rand) string {
	terms := [...]string{"alpha", "beta", "gamma", "delta", "epsilon", "missing"}
	query := make([]string, generator.IntN(5)+1)
	for position := range query {
		query[position] = terms[generator.IntN(len(terms))]
	}
	return strings.Join(query, " ")
}
