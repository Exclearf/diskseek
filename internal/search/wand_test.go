package search

import (
	"context"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/segment"
)

func TestSelectWANDPivot(t *testing.T) {
	tests := []struct {
		name          string
		bounds        []float64
		cursors       []wandCursor
		threshold     float64
		wantDocument  index.DocumentID
		wantPreceding []wandCursor
		wantFound     bool
	}{
		{
			name:      "complete bound equals threshold",
			bounds:    []float64{0.5, 0.5},
			cursors:   []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold: 1,
		},
		{
			name:      "complete bound one ULP below threshold",
			bounds:    []float64{0.5, 0.5},
			cursors:   []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold: math.Nextafter(1, math.Inf(1)),
		},
		{
			name:          "complete bound one ULP above threshold",
			bounds:        []float64{math.Nextafter(1, math.Inf(1))},
			cursors:       []wandCursor{{termIndex: 0, documentID: 2}},
			threshold:     1,
			wantDocument:  2,
			wantPreceding: []wandCursor{},
			wantFound:     true,
		},
		{
			name:          "equal prefix followed by larger prefix",
			bounds:        []float64{0.25, 0.75, 0.5},
			cursors:       []wandCursor{{termIndex: 2, documentID: 8}, {termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 2}},
			threshold:     1,
			wantDocument:  8,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 2}, {termIndex: 1, documentID: 5}},
			wantFound:     true,
		},
		{
			name:          "pivot not aligned with first cursor",
			bounds:        []float64{0.4, 0.8, 0.7},
			cursors:       []wandCursor{{termIndex: 2, documentID: 5}, {termIndex: 0, documentID: 2}, {termIndex: 1, documentID: 5}},
			threshold:     1,
			wantDocument:  5,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 2}},
			wantFound:     true,
		},
		{
			name:          "equal document IDs use canonical term order",
			bounds:        []float64{0.4, 0.6},
			cursors:       []wandCursor{{termIndex: 1, documentID: 5}, {termIndex: 0, documentID: 5}},
			threshold:     0.5,
			wantDocument:  5,
			wantPreceding: []wandCursor{{termIndex: 0, documentID: 5}},
			wantFound:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := diskQueryPlan{terms: make([]diskQueryTerm, len(test.bounds))}
			for termIndex, bound := range test.bounds {
				plan.terms[termIndex].upperBound = bound
			}

			pivot, found := selectWANDPivot(&plan, slices.Clone(test.cursors), test.threshold)
			if found != test.wantFound {
				t.Fatalf("pivot found = %t, want %t", found, test.wantFound)
			}
			if !found {
				return
			}
			if pivot.documentID != test.wantDocument || !slices.Equal(pivot.preceding, test.wantPreceding) {
				t.Fatalf("pivot = %+v, want document %d preceding %+v", pivot, test.wantDocument, test.wantPreceding)
			}
		})
	}
}

func TestExecuteWAND(t *testing.T) {
	t.Run("equal bound terminates", func(t *testing.T) {
		const input = "d0\tterm\nd1\tterm\nd2\tterm\n"
		disk, logical := buildWANDTestIndex(t, input)
		plan, err := buildDiskQueryPlan(disk, "term")
		if err != nil {
			t.Fatal(err)
		}

		got, stats, err := executeWAND(disk, plan, 1)
		if err != nil {
			t.Fatal(err)
		}
		want, err := referenceSearch(&logical, "term", 1)
		if err != nil {
			t.Fatal(err)
		}
		checkWANDResults(t, got, want, &logical)

		wantStats := wandStats{
			NextCalls:        1,
			BlockHeadersRead: 1,
			BlocksDecoded:    1,
			PostingsDecoded:  3,
			BytesRequested:   14,
			CandidatesScored: 1,
			ThresholdChanges: 1,
		}
		if stats != wantStats {
			t.Fatalf("WAND stats = %+v, want %+v", stats, wantStats)
		}
	})

	t.Run("unaligned pivot advances highest IDF term", func(t *testing.T) {
		const input = "d0\tseed seed seed seed\n" +
			"d1\tcommon\n" +
			"d2\tcommon rare\n" +
			"d3\tcommon pivot\n" +
			"d4\tcommon\n"
		disk, logical := buildWANDTestIndex(t, input)
		plan, err := buildDiskQueryPlan(disk, "seed common rare pivot")
		if err != nil {
			t.Fatal(err)
		}

		got, stats, err := executeWAND(disk, plan, 1)
		if err != nil {
			t.Fatal(err)
		}
		want, err := referenceSearch(&logical, "seed common rare pivot", 1)
		if err != nil {
			t.Fatal(err)
		}
		checkWANDResults(t, got, want, &logical)

		for _, term := range plan.terms {
			var want uint64
			if term.term == "rare" {
				want = 1
			}
			if got := term.cursor.Stats().AdvanceCalls; got != want {
				t.Fatalf("%q advance calls = %d, want %d", term.term, got, want)
			}
		}
		if stats.PivotSelections != 1 || stats.CandidatesScored != 1 || stats.ThresholdChanges != 1 {
			t.Fatalf("WAND stats = %+v; want one pivot, candidate, and threshold change", stats)
		}
	})

	t.Run("replacement raises threshold", func(t *testing.T) {
		const input = "d0\tterm filler\n" +
			"d1\tterm\n"
		disk, _ := buildWANDTestIndex(t, input)

		_, stats, err := searchWAND(disk, "term", 1)
		if err != nil {
			t.Fatal(err)
		}
		if stats.CandidatesScored != 2 || stats.PivotSelections != 1 || stats.ThresholdChanges != 2 {
			t.Fatalf("WAND stats = %+v; want two candidates, one pivot, and two threshold changes", stats)
		}
	})
}

func TestWANDCandidatePruning(t *testing.T) {
	t.Run("avoids scoring", func(t *testing.T) {
		const input = "d0\trare rare rare rare common\n" +
			"d1\tcommon\n" +
			"d2\tcommon\n" +
			"d3\tcommon\n" +
			"d4\tcommon\n" +
			"d5\tcommon\n"
		disk, _ := buildWANDTestIndex(t, input)

		exhaustive, exhaustiveStats, err := searchDAAT(disk, "rare common", 1)
		if err != nil {
			t.Fatal(err)
		}
		wand, wandStats, err := searchWAND(disk, "rare common", 1)
		if err != nil {
			t.Fatal(err)
		}

		if !equalResultBits(wand, exhaustive) {
			t.Fatalf("WAND results = %+v, exhaustive results = %+v", wand, exhaustive)
		}
		if exhaustiveStats.CandidatesScored != 6 || wandStats.CandidatesScored != 1 {
			t.Fatalf("candidates scored = exhaustive %d, WAND %d; want 6, 1", exhaustiveStats.CandidatesScored, wandStats.CandidatesScored)
		}
	})

	t.Run("loose bound scores every candidate", func(t *testing.T) {
		const input = "d0\tterm filler\n" +
			"d1\tterm filler\n" +
			"d2\tterm filler\n"
		disk, _ := buildWANDTestIndex(t, input)

		exhaustive, exhaustiveStats, err := searchDAAT(disk, "term", 1)
		if err != nil {
			t.Fatal(err)
		}
		wand, wandStats, err := searchWAND(disk, "term", 1)
		if err != nil {
			t.Fatal(err)
		}

		if !equalResultBits(wand, exhaustive) {
			t.Fatalf("WAND results = %+v, exhaustive results = %+v", wand, exhaustive)
		}
		if exhaustiveStats.CandidatesScored != 3 || wandStats.CandidatesScored != 3 {
			t.Fatalf("candidates scored = exhaustive %d, WAND %d; want 3, 3", exhaustiveStats.CandidatesScored, wandStats.CandidatesScored)
		}
	})
}

func buildWANDTestIndex(t *testing.T, input string) (*indexfile.Index, index.Index) {
	t.Helper()
	logical, err := index.Build(corpus.NewTSVReader(strings.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "index")
	err = segment.BuildIndex(
		context.Background(),
		corpus.NewTSVReader(strings.NewReader(input)),
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
	return openDiskTestIndex(t, destination), logical
}

func checkWANDResults(t *testing.T, got, want []result, logical *index.Index) {
	t.Helper()
	if !equalResultBits(got, want) {
		t.Fatalf("WAND results = %+v, want %+v", got, want)
	}
	for position := range got {
		wantExternalID := logical.Documents[got[position].DocumentID].ExternalID
		if got[position].ExternalID != wantExternalID {
			t.Fatalf("result %d external ID = %q, want %q", position, got[position].ExternalID, wantExternalID)
		}
	}
}
