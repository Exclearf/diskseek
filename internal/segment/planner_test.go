package segment

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func TestPlanMergePass(t *testing.T) {
	paths := []string{"r0", "r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8", "r9"}
	got, err := planMergePass(paths, 3)
	if err != nil {
		t.Fatal(err)
	}

	want := []mergeGroup{
		{groupIndex: 0, inputPaths: paths[0:3]},
		{groupIndex: 1, inputPaths: paths[3:6]},
		{groupIndex: 2, inputPaths: paths[6:9]},
		{groupIndex: 3, inputPaths: paths[9:10]},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %#v, want %#v", got, want)
	}
}

func TestPlanMergePassRejectsFanInBelowTwo(t *testing.T) {
	if _, err := planMergePass([]string{"r0", "r1"}, 1); err == nil {
		t.Fatal("planMergePass() error = nil")
	}
}

func TestMergeRunPass(t *testing.T) {
	directory := t.TempDir()
	runs := [][]byte{
		encodeMergeTestRun(t, runHeader{documentCount: 1}, []mergeTestTerm{
			{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}}},
		}),
		encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, []mergeTestTerm{
			{term: "banana", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
		}),
		encodeMergeTestRun(t, runHeader{firstDocumentID: 2, documentCount: 1}, []mergeTestTerm{
			{term: "apple", postings: []index.Posting{{DocumentID: 2, Frequency: 1}}},
		}),
		encodeMergeTestRun(t, runHeader{firstDocumentID: 3, documentCount: 1}, []mergeTestTerm{
			{term: "yak", postings: []index.Posting{{DocumentID: 3, Frequency: 1}}},
		}),
	}

	names := []string{"r0", "r1", "r2", "r3"}
	paths := make([]string, len(runs))
	for i, run := range runs {
		paths[i] = filepath.Join(directory, names[i])
		if err := os.WriteFile(paths[i], run, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	gotPaths, gotStats, err := mergeRunPass(directory, paths, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotPaths) != 2 {
		t.Fatalf("successor count = %d, want 2", len(gotPaths))
	}

	wantMerged := encodeMergeTestRun(t, runHeader{documentCount: 3}, []mergeTestTerm{
		{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}, {DocumentID: 2, Frequency: 1}}},
		{term: "banana", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
	})
	gotMerged, err := os.ReadFile(gotPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMerged, wantMerged) {
		t.Fatal("merged successor does not match expected bytes")
	}
	if gotPaths[1] != paths[3] {
		t.Fatalf("carried path = %q, want %q", gotPaths[1], paths[3])
	}

	wantStats := []mergeGroupStats{
		{
			passIndex:   0,
			groupIndex:  0,
			inputCount:  3,
			inputBytes:  uint64(len(runs[0]) + len(runs[1]) + len(runs[2])),
			outputBytes: uint64(len(wantMerged)),
		},
		{
			passIndex:   0,
			groupIndex:  1,
			inputCount:  1,
			inputBytes:  uint64(len(runs[3])),
			outputBytes: uint64(len(runs[3])),
		},
	}
	if !reflect.DeepEqual(gotStats, wantStats) {
		t.Fatalf("statistics = %#v, want %#v", gotStats, wantStats)
	}
}
