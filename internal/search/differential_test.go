package search

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
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

func TestDiskExecutorsMatchReference(t *testing.T) {
	authored, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	checkDiskExecutorParity(t, authored, []differentialQuery{
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

func TestGeneratedDiskExecutorsMatchReference(t *testing.T) {
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
			checkDiskExecutorParity(t, input, queries)
		})
	}
}

func TestSearchModelParityMatrix(t *testing.T) {
	for _, seed := range []uint64{13, 29, 47} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			model := generateSearchModel(fixedSearchModelConfig(seed))
			checkDiskExecutorParity(t, model.input, model.queries)
		})
	}
}

func TestSearchModelMetamorphicQueries(t *testing.T) {
	model := generateSearchModel(fixedSearchModelConfig(13))
	logical, err := index.Build(corpus.NewTSVReader(bytes.NewReader(model.input)))
	if err != nil {
		t.Fatal(err)
	}

	canonical, err := referenceSearch(&logical, "common rare tie", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"tie common rare", "common rare tie tie"} {
		got, err := referenceSearch(&logical, query, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !equalResultBits(got, canonical) {
			t.Fatalf("query %q changed canonical results", query)
		}
	}

	small, err := referenceSearch(&logical, "common rare tie", 2)
	if err != nil {
		t.Fatal(err)
	}
	large, err := referenceSearch(&logical, "common rare tie", 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(small) > len(large) || !equalResultBits(small, large[:len(small)]) {
		t.Fatal("top-2 results are not a prefix of top-7 results")
	}

	destination := buildDifferentialIndex(t, model.input, indexfile.PostingsCodecVByte, 1)
	searchOnce := func() []result {
		idx, err := indexfile.Open(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := idx.Close(); err != nil {
				t.Error(err)
			}
		}()
		checkDiskLogicalIndex(t, idx, &logical)

		results, _, err := searchWAND(idx, "common rare tie", 5)
		if err != nil {
			t.Fatal(err)
		}
		return results
	}
	beforeClose := searchOnce()
	afterReopen := searchOnce()
	if !equalResultBits(beforeClose, afterReopen) {
		t.Fatal("close and reopen changed query results")
	}
}

func checkDiskExecutorParity(t *testing.T, input []byte, queries []differentialQuery) {
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
		for _, layout := range []struct {
			name        string
			flushTarget uint64
		}{
			{name: "one-run", flushTarget: math.MaxUint64},
			{name: "many-run", flushTarget: 1},
		} {
			t.Run(codec.name+"/"+layout.name, func(t *testing.T) {
				destination := buildDifferentialIndex(t, input, codec.value, layout.flushTarget)
				disk := openDiskTestIndex(t, destination)
				checkDiskLogicalIndex(t, disk, &logical)

				for queryIndex, query := range queries {
					t.Run(fmt.Sprintf("query-%d", queryIndex), func(t *testing.T) {
						want, err := referenceSearch(&logical, query.query, query.k)
						if err != nil {
							t.Fatal(err)
						}
						exhaustive, exhaustiveStats, err := searchDAAT(disk, query.query, query.k)
						if err != nil {
							t.Fatal(err)
						}
						if !equalResultBits(exhaustive, want) {
							t.Fatalf("searchDAAT(%q, %d) = %+v, want %+v", query.query, query.k, exhaustive, want)
						}

						wand, wandStats, err := searchWAND(disk, query.query, query.k)
						if err != nil {
							t.Fatal(err)
						}
						if !equalResultBits(wand, exhaustive) {
							t.Fatalf("searchWAND(%q, %d) = %+v, want %+v", query.query, query.k, wand, exhaustive)
						}
						if wandStats.CandidatesScored > exhaustiveStats.CandidatesScored {
							t.Fatalf("WAND scored %d candidates, exhaustive DAAT scored %d", wandStats.CandidatesScored, exhaustiveStats.CandidatesScored)
						}

						postings, candidates, err := expectedDAATWork(logical.Postings, query.query, query.k)
						if err != nil {
							t.Fatal(err)
						}
						if exhaustiveStats.PostingsDecoded != postings || exhaustiveStats.NextCalls != postings ||
							exhaustiveStats.CandidatesScored != candidates || exhaustiveStats.AdvanceCalls != 0 {
							t.Fatalf("stats = %+v, want postings/next %d, candidates %d, advances 0", exhaustiveStats, postings, candidates)
						}
						requestedBytes[codec.value] += exhaustiveStats.BytesRequested
					})
				}
			})
		}
	}
	if requestedBytes[indexfile.PostingsCodecRaw] == requestedBytes[indexfile.PostingsCodecVByte] {
		t.Fatalf("raw and VByte requested the same %d posting bytes", requestedBytes[indexfile.PostingsCodecRaw])
	}
}

func checkDiskLogicalIndex(t *testing.T, disk *indexfile.Index, logical *index.Index) {
	t.Helper()
	if disk.DocumentsWithTerms() != logical.DocumentsWithTerms {
		t.Fatalf("documents with terms = %d, want %d", disk.DocumentsWithTerms(), logical.DocumentsWithTerms)
	}
	for documentID, document := range logical.Documents {
		id := index.DocumentID(documentID)
		if length := disk.DocumentLength(id); length != document.Length {
			t.Fatalf("document %d length = %d, want %d", id, length, document.Length)
		}
		externalID, err := disk.ExternalID(id)
		if err != nil {
			t.Fatal(err)
		}
		if externalID != document.ExternalID {
			t.Fatalf("document %d external ID = %q, want %q", id, externalID, document.ExternalID)
		}
	}
	for term, want := range logical.Postings {
		cursor, found, err := disk.Postings(term)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("term %q is missing", term)
		}

		got := make([]index.Posting, 0, len(want))
		for {
			posting, valid := cursor.Current()
			if !valid {
				break
			}
			got = append(got, posting)
			if _, err := cursor.Next(); err != nil {
				t.Fatal(err)
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("term %q postings = %+v, want %+v", term, got, want)
		}
	}
}

func buildDifferentialIndex(
	t *testing.T,
	input []byte,
	codec indexfile.PostingsCodec,
	flushTarget uint64,
) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "index")
	err := segment.BuildIndex(
		context.Background(),
		corpus.NewTSVReader(bytes.NewReader(input)),
		destination,
		segment.BuildOptions{
			FlushTarget:        flushTarget,
			MergeFanIn:         2,
			MergeWorkers:       2,
			Codec:              codec,
			TemporaryDirectory: t.TempDir(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return destination
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
