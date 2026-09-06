package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"testing"

	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
)

func TestDigestResultsCoversResultIdentity(t *testing.T) {
	results := []search.MeasuredResult{
		{DocumentID: 1, ExternalID: "one", Score: 1},
		{DocumentID: 2, ExternalID: "two", Score: 2},
	}
	want := digestResults(results)

	reordered := []search.MeasuredResult{results[1], results[0]}
	if digestResults(reordered) == want {
		t.Fatal("result order did not change digest")
	}
	changedID := append([]search.MeasuredResult(nil), results...)
	changedID[0].ExternalID = "other"
	if digestResults(changedID) == want {
		t.Fatal("external ID did not change digest")
	}
	changedScore := append([]search.MeasuredResult(nil), results...)
	changedScore[0].Score = math.Nextafter(changedScore[0].Score, 2)
	if digestResults(changedScore) == want {
		t.Fatal("score bits did not change digest")
	}
}

func TestSummarizeBuild(t *testing.T) {
	observation := buildObservation{
		Codec:           "vbyte",
		Repetition:      2,
		ElapsedNS:       2e9,
		CorpusBytes:     20 * bytesPerMiB,
		Documents:       100,
		Tokens:          200,
		PostingsCount:   1024 * 1024,
		FinalIndexBytes: 8 * bytesPerMiB,
		PeakRSSBytes:    32 * bytesPerMiB,
	}
	want := buildResult{
		Codec:                 "vbyte",
		Repetition:            2,
		ElapsedSeconds:        2,
		DocumentsPerSecond:    50,
		TokensPerSecond:       100,
		PostingsPerSecond:     512 * 1024,
		InputMiBPerSecond:     10,
		IndexMiB:              8,
		IndexBytesPerPosting:  8,
		PeakResidentMemoryMiB: 32,
	}
	if got := summarizeBuild(observation); got != want {
		t.Fatalf("summarizeBuild() = %+v, want %+v", got, want)
	}
}

func TestRunQueriesWritesMeasuredRows(t *testing.T) {
	idx, err := indexfile.Open("../../internal/indexfile/testdata/golden-v1/vbyte")
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	queries := []string{"go", "missing", string([]byte{0xff})}
	var output bytes.Buffer
	err = runQueries(context.Background(), idx, queries, queryOptions{
		repetition:   2,
		executor:     search.ExecutorDAAT,
		executorName: "daat",
		limit:        3,
		warmup:       1,
	}, &output)
	if err == nil {
		t.Fatal("runQueries() error = nil with a failed query")
	}

	decoder := json.NewDecoder(&output)
	records := make([]queryObservation, 3)
	for row := range records {
		if err := decoder.Decode(&records[row]); err != nil {
			t.Fatalf("decode row %d: %v", row+1, err)
		}
	}
	var extra queryObservation
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("decode after final row: %v", err)
	}

	if records[0].Repetition != 2 ||
		records[0].Codec != "vbyte" ||
		records[0].Executor != "daat" ||
		records[0].Limit != 3 ||
		records[0].Workers != 1 {
		t.Fatalf("row identity = %+v", records[0])
	}
	for row := range records {
		if records[row].QueryOrdinal != row+1 {
			t.Fatalf("row %d ordinal = %d", row+1, records[row].QueryOrdinal)
		}
	}

	if records[0].Status != "ok" ||
		records[0].ResultCount != 2 ||
		records[0].ResultDigest == "" ||
		records[0].MatchedTerms != 1 ||
		records[0].CandidatesScored != 2 {
		t.Fatalf("short-result row = %+v", records[0])
	}
	if records[1].Status != "ok" ||
		records[1].ResultCount != 0 ||
		records[1].ResultDigest == "" ||
		records[1].MatchedTerms != 0 ||
		records[1].CandidatesScored != 0 ||
		records[1].PostingsDecoded != 0 {
		t.Fatalf("zero-result row = %+v", records[1])
	}
	if records[2].Status != "search_error" ||
		records[2].ResultCount != 0 ||
		records[2].ResultDigest != "" {
		t.Fatalf("error row = %+v", records[2])
	}
}
