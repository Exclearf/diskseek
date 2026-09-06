package simplewiki

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
)

func TestConvert(t *testing.T) {
	oversized := strings.Repeat("x", corpus.MaxRecordBytes)
	input := strings.Join([]string{
		`{"index":{"_id":7}}`,
		`{"page_id":7,"namespace":0,"title":"Honey bee","text":"Bees\nmake\thoney.","opening_text":"Bees make honey."}`,
		`{"index":{"_id":8}}`,
		`{"page_id":8,"namespace":1,"title":"Talk:Honey bee","text":"Discussion"}`,
		`{"index":{"_id":9}}`,
		`{"page_id":9,"namespace":0,"title":"Large","text":"` + oversized + `"}`,
	}, "\n")

	var corpusOutput bytes.Buffer
	var catalogOutput bytes.Buffer
	stats, err := Convert(t.Context(), strings.NewReader(input), &corpusOutput, &catalogOutput)
	if err != nil {
		t.Fatal(err)
	}
	if want := "7\tHoney bee Bees make honey.\n"; corpusOutput.String() != want {
		t.Fatalf("corpus = %q, want %q", corpusOutput.String(), want)
	}
	if want := (Stats{Documents: 1, SkippedNamespaces: 1, SkippedOversized: 1}); stats != want {
		t.Fatalf("stats = %+v, want %+v", stats, want)
	}

	var record catalogRecord
	if err := json.NewDecoder(&catalogOutput).Decode(&record); err != nil {
		t.Fatal(err)
	}
	want := catalogRecord{
		ExternalID: "7",
		Title:      "Honey bee",
		Preview:    "Bees make honey.",
		SourceURL:  "https://simple.wikipedia.org/wiki/Honey_bee",
	}
	if record != want {
		t.Fatalf("catalog record = %+v, want %+v", record, want)
	}
}
