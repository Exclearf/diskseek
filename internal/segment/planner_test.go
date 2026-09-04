package segment

import (
	"bytes"
	"fmt"
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

	paths := writeMergeTestRuns(t, directory, runs)

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

func TestMergeRunPassRemovesFailedSuccessor(t *testing.T) {
	directory := t.TempDir()
	runs := [][]byte{
		encodeMergeTestRun(t, runHeader{documentCount: 1}, []mergeTestTerm{
			{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}}},
		}),
		encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, []mergeTestTerm{
			{term: "banana", postings: []index.Posting{{DocumentID: 1, Frequency: 1}}},
		}),
	}
	runs[1] = append(runs[1], 0)
	paths := writeMergeTestRuns(t, directory, runs)

	if _, _, err := mergeRunPass(directory, paths, 2, 0); err == nil {
		t.Fatal("mergeRunPass() error = nil")
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat source %q: %v", path, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(paths) {
		t.Fatalf("directory entries = %d, want %d source runs", len(entries), len(paths))
	}
}

func TestMergeRunsProducesSameBytesAcrossFanIn(t *testing.T) {
	const runCount = 10
	runs := make([][]byte, runCount)
	postings := make([]index.Posting, runCount)
	for documentID := range runCount {
		posting := index.Posting{DocumentID: index.DocumentID(documentID), Frequency: 1}
		postings[documentID] = posting
		runs[documentID] = encodeMergeTestRun(t, runHeader{
			firstDocumentID: index.DocumentID(documentID),
			documentCount:   1,
		}, []mergeTestTerm{{
			term:     "shared",
			postings: []index.Posting{posting},
		}})
	}
	paths := writeMergeTestRuns(t, t.TempDir(), runs)

	want := encodeMergeTestRun(t, runHeader{documentCount: runCount}, []mergeTestTerm{
		{term: "shared", postings: postings},
	})

	tests := []struct {
		name        string
		fanIn       int
		groupCounts []int
	}{
		{name: "fan-in 2", fanIn: 2, groupCounts: []int{5, 3, 2, 1}},
		{name: "fan-in 3", fanIn: 3, groupCounts: []int{4, 2, 1}},
		{name: "one pass", fanIn: 10, groupCounts: []int{1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path, stats, err := mergeRuns(t.TempDir(), paths, test.fanIn)
			if err != nil {
				t.Fatal(err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("final run does not match expected bytes")
			}

			statIndex := 0
			for passIndex, groupCount := range test.groupCounts {
				for groupIndex := range groupCount {
					if statIndex == len(stats) {
						t.Fatalf("statistics ended before pass %d group %d", passIndex, groupIndex)
					}
					stat := stats[statIndex]
					if stat.passIndex != passIndex || stat.groupIndex != groupIndex {
						t.Fatalf("statistics[%d] identifies pass %d group %d, want pass %d group %d", statIndex, stat.passIndex, stat.groupIndex, passIndex, groupIndex)
					}
					statIndex++
				}
			}
			if statIndex != len(stats) {
				t.Fatalf("statistics count = %d, want %d", len(stats), statIndex)
			}
		})
	}
}

func TestMergeRunsCreatesCanonicalEmptyRun(t *testing.T) {
	path, stats, err := mergeRuns(t.TempDir(), nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 0 {
		t.Fatalf("statistics count = %d, want 0", len(stats))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := encodeMergeTestRun(t, runHeader{}, nil)
	if !bytes.Equal(got, want) {
		t.Fatal("empty run does not match canonical bytes")
	}
}

func TestMergeRunsValidatesAndAdoptsSoleRun(t *testing.T) {
	directory := t.TempDir()
	run := encodeMergeTestRun(t, runHeader{documentCount: 1}, []mergeTestTerm{
		{term: "apple", postings: []index.Posting{{DocumentID: 0, Frequency: 1}}},
	})
	path := writeMergeTestRuns(t, directory, [][]byte{run})[0]

	gotPath, stats, err := mergeRuns(t.TempDir(), []string{path}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path {
		t.Fatalf("adopted path = %q, want %q", gotPath, path)
	}
	if len(stats) != 0 {
		t.Fatalf("statistics count = %d, want 0", len(stats))
	}

	corruptPath := filepath.Join(directory, "corrupt")
	if err := os.WriteFile(corruptPath, append(run, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mergeRuns(t.TempDir(), []string{corruptPath}, 2); err == nil {
		t.Fatal("mergeRuns() error = nil for corrupt sole run")
	}
}

func TestMergeRunsRejectsFanInBeforeCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	if _, _, err := mergeRuns(directory, nil, 1); err == nil {
		t.Fatal("mergeRuns() error = nil")
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("output count = %d, want 0", len(entries))
	}
}

func writeMergeTestRuns(t *testing.T, directory string, runs [][]byte) []string {
	t.Helper()

	paths := make([]string, len(runs))
	for i, run := range runs {
		paths[i] = filepath.Join(directory, fmt.Sprintf("r%02d", len(runs)-i))
		if err := os.WriteFile(paths[i], run, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}
