package search

import (
	"context"
	"math"
	"math/rand"
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

	plan, err := buildDiskQueryPlan(context.Background(), idx, "SEARCH missing GO go")
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

func TestSelectedUpperBoundUsesCanonicalTermOrder(t *testing.T) {
	plan := diskQueryPlan{terms: []diskQueryTerm{
		{term: "a", upperBound: 0.1},
		{term: "b", upperBound: 0.2},
		{term: "c", upperBound: 0.3},
		{term: "d", upperBound: 0.4},
	}}
	cursorOrder := []int{2, 1, 0}
	selected := make([]bool, len(plan.terms))
	for _, termIndex := range cursorOrder {
		selected[termIndex] = true
	}

	got := plan.selectedUpperBound(selected)
	var want float64
	for _, term := range plan.terms[:3] {
		want += term.upperBound
	}
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("selected upper bound bits = %#x, want %#x", math.Float64bits(got), math.Float64bits(want))
	}

	var cursorOrderSum float64
	for _, termIndex := range cursorOrder {
		cursorOrderSum += plan.terms[termIndex].upperBound
	}
	if math.Float64bits(got) == math.Float64bits(cursorOrderSum) {
		t.Fatal("test values do not distinguish canonical and cursor order")
	}
}

func TestSelectedUpperBoundDominatesCanonicalScore(t *testing.T) {
	const generatedCases = 10_000

	random := rand.New(rand.NewSource(0))
	for testCase := range generatedCases {
		termCount := random.Intn(8) + 1
		plan := diskQueryPlan{terms: make([]diskQueryTerm, termCount)}
		contributions := make([]float64, termCount)
		selectedTerms := make([]bool, termCount)
		selectedPositions := make([]int, 0, termCount)
		for termIndex := range plan.terms {
			bound := random.Float64()*44 + 1e-12
			plan.terms[termIndex].upperBound = bound
			if termIndex != 0 && random.Intn(2) == 0 {
				continue
			}
			selectedTerms[termIndex] = true
			selectedPositions = append(selectedPositions, termIndex)
			if testCase%2 == 0 {
				contributions[termIndex] = math.Nextafter(bound, 0)
			} else {
				contributions[termIndex] = random.Float64() * bound
			}
		}

		random.Shuffle(len(selectedPositions), func(left, right int) {
			selectedPositions[left], selectedPositions[right] = selectedPositions[right], selectedPositions[left]
		})
		selected := make([]bool, termCount)
		for _, termIndex := range selectedPositions {
			selected[termIndex] = true
		}

		var score, wantBound float64
		for termIndex, contribution := range contributions {
			if selectedTerms[termIndex] {
				score += contribution
				wantBound += plan.terms[termIndex].upperBound
			}
		}
		bound := plan.selectedUpperBound(selected)
		if math.Float64bits(bound) != math.Float64bits(wantBound) {
			t.Fatalf("case %d: bound bits = %#x, want %#x", testCase, math.Float64bits(bound), math.Float64bits(wantBound))
		}
		if score > bound {
			t.Fatalf("case %d: score %v exceeds bound %v", testCase, score, bound)
		}
	}
}
