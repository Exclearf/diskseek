package search

import (
	"math"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestPrepareQuery(t *testing.T) {
	got, err := prepareQuery("Straße ALPHA strasse")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"alpha", "strasse"}
	if !slices.Equal(got, want) {
		t.Fatalf("prepareQuery() = %q, want %q", got, want)
	}
}

func TestBuildDiskQueryPlanUsesPreparedTerms(t *testing.T) {
	idx, err := indexfile.Open(filepath.Join("..", "indexfile", "testdata", "golden-v1", "vbyte"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := idx.Close(); err != nil {
			t.Error(err)
		}
	})

	plan, err := buildDiskQueryPlan(idx, "SEARCH missing GO go")
	if err != nil {
		t.Fatal(err)
	}
	if plan.averageDocumentLength != 2.5 {
		t.Fatalf("average document length = %v, want 2.5", plan.averageDocumentLength)
	}

	want := []struct {
		term              string
		documentFrequency uint64
		maxTermFrequency  uint32
	}{
		{term: "go", documentFrequency: 2, maxTermFrequency: 3},
		{term: "search", documentFrequency: 1, maxTermFrequency: 1},
	}
	if len(plan.terms) != len(want) {
		t.Fatalf("planned terms = %d, want %d", len(plan.terms), len(want))
	}
	for position, wantTerm := range want {
		got := plan.terms[position]
		if got.term != wantTerm.term {
			t.Fatalf("planned term %d = %q, want %q", position, got.term, wantTerm.term)
		}
		if got.cursor.DocumentFrequency() != wantTerm.documentFrequency {
			t.Fatalf(
				"%q document frequency = %d, want %d",
				got.term,
				got.cursor.DocumentFrequency(),
				wantTerm.documentFrequency,
			)
		}
		wantIDF := bm25IDF(idx.DocumentsWithTerms(), wantTerm.documentFrequency)
		if math.Float64bits(got.idf) != math.Float64bits(wantIDF) {
			t.Fatalf("%q IDF = %v, want %v", got.term, got.idf, wantIDF)
		}
		wantUpperBound := bm25TermUpperBound(wantIDF, wantTerm.maxTermFrequency, plan.averageDocumentLength)
		if math.Float64bits(got.upperBound) != math.Float64bits(wantUpperBound) {
			t.Fatalf("%q upper bound = %v, want %v", got.term, got.upperBound, wantUpperBound)
		}
	}
}
