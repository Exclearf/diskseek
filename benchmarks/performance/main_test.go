package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"math"
	"strconv"
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

func TestRunQueriesWritesMeasuredRows(t *testing.T) {
	idx, err := indexfile.Open("../../internal/indexfile/testdata/golden-v1/vbyte")
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	queries := []string{"go", "missing", string([]byte{0xff})}
	var output bytes.Buffer
	err = runQueries(context.Background(), idx, queries, queryOptions{
		runID:        "test-run",
		repetition:   2,
		executor:     search.ExecutorDAAT,
		executorName: "daat",
		limit:        3,
		warmup:       1,
	}, &output)
	if err == nil {
		t.Fatal("runQueries() error = nil with a failed query")
	}

	reader := csv.NewReader(bytes.NewReader(output.Bytes()))
	reader.Comma = '\t'
	reader.FieldsPerRecord = len(queryHeader)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("TSV rows = %d, want one header and three measured queries", len(records))
	}

	column := func(name string) int {
		t.Helper()
		for position, candidate := range queryHeader {
			if candidate == name {
				return position
			}
		}
		t.Fatalf("missing column %q", name)
		return 0
	}
	if records[1][column("run_id")] != "test-run" ||
		records[1][column("repetition")] != "2" ||
		records[1][column("codec")] != "vbyte" ||
		records[1][column("executor")] != "daat" ||
		records[1][column("k")] != "3" ||
		records[1][column("workers")] != "1" {
		t.Fatalf("row identity = %v", records[1][:7])
	}
	for row := 1; row < len(records); row++ {
		if records[row][column("query_ordinal")] != strconv.Itoa(row) {
			t.Fatalf("row %d ordinal = %q", row, records[row][column("query_ordinal")])
		}
	}

	if records[1][column("status")] != "ok" ||
		records[1][column("result_count")] != "2" ||
		records[1][column("result_digest")] == "" ||
		records[1][column("matched_terms")] != "1" ||
		records[1][column("candidates_scored")] != "2" {
		t.Fatalf("short-result row = %v", records[1])
	}
	if records[2][column("status")] != "ok" ||
		records[2][column("result_count")] != "0" ||
		records[2][column("result_digest")] == "" ||
		records[2][column("matched_terms")] != "0" ||
		records[2][column("candidates_scored")] != "0" ||
		records[2][column("postings_decoded")] != "0" {
		t.Fatalf("zero-result row = %v", records[2])
	}
	if records[3][column("status")] != "search_error" ||
		records[3][column("result_count")] != "0" ||
		records[3][column("result_digest")] != "" {
		t.Fatalf("error row = %v", records[3])
	}
}
