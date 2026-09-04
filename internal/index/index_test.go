package index

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
)

func TestBuild(t *testing.T) {
	input, err := os.Open("testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()

	got, err := Build(corpus.NewTSVReader(input))
	if err != nil {
		t.Fatal(err)
	}

	want := Index{
		Documents: []DocumentMeta{
			{ExternalID: "shared", Length: 3},
			{ExternalID: "shared", Length: 3},
			{ExternalID: "repeat", Length: 4},
			{ExternalID: "unicode", Length: 2},
			{ExternalID: "empty", Length: 0},
		},
		Postings: map[string][]Posting{
			"café":    {{DocumentID: 3, Frequency: 1}},
			"disk":    {{DocumentID: 2, Frequency: 4}},
			"fast":    {{DocumentID: 0, Frequency: 1}},
			"index":   {{DocumentID: 0, Frequency: 1}, {DocumentID: 1, Frequency: 1}},
			"search":  {{DocumentID: 0, Frequency: 1}, {DocumentID: 1, Frequency: 1}},
			"small":   {{DocumentID: 1, Frequency: 1}},
			"strasse": {{DocumentID: 3, Frequency: 1}},
		},
		DocumentsWithTerms: 4,
		TotalLength:        12,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() = %#v, want %#v", got, want)
	}
}

func TestBuildReturnsCorpusError(t *testing.T) {
	input := strings.NewReader("valid\ttext\nbroken\n")

	_, err := Build(corpus.NewTSVReader(input))
	if !errors.Is(err, corpus.ErrMalformedRecord) {
		t.Fatalf("Build() error = %v, want %v", err, corpus.ErrMalformedRecord)
	}
}
