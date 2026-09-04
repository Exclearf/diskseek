package segment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
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

	gotPaths, gotStats, err := mergeRunPass(context.Background(), directory, paths, 3, 1, 0)
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
	for _, path := range paths[:3] {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat merged source %q: %v, want not exist", path, err)
		}
	}
	if _, err := os.Stat(paths[3]); err != nil {
		t.Fatalf("stat carried source %q: %v", paths[3], err)
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

func TestMergeRunPassPreservesSourcesWhenOutputCreationFails(t *testing.T) {
	runs := [][]byte{
		encodeMergeTestRun(t, runHeader{documentCount: 1}, nil),
		encodeMergeTestRun(t, runHeader{firstDocumentID: 1, documentCount: 1}, nil),
	}
	paths := writeMergeTestRuns(t, t.TempDir(), runs)
	missingDirectory := filepath.Join(t.TempDir(), "missing")

	if _, _, err := mergeRunPass(context.Background(), missingDirectory, paths, 2, 1, 0); err == nil {
		t.Fatal("mergeRunPass() error = nil")
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat preserved source %q: %v", path, err)
		}
	}
}

func TestMergeRunPassRollsBackCompletedSuccessors(t *testing.T) {
	directory := t.TempDir()
	keepPath := filepath.Join(directory, "keep")
	if err := os.WriteFile(keepPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	runs := make([][]byte, 4)
	for documentID := range runs {
		runs[documentID] = encodeMergeTestRun(t, runHeader{
			firstDocumentID: index.DocumentID(documentID),
			documentCount:   1,
		}, nil)
	}
	runs[3] = append(runs[3], 0)
	paths := writeMergeTestRuns(t, directory, runs)

	if _, _, err := mergeRunPass(context.Background(), directory, paths, 2, 1, 0); err == nil {
		t.Fatal("mergeRunPass() error = nil")
	}
	for _, path := range append(paths, keepPath) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("stat preserved file %q: %v", path, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(paths)+1 {
		t.Fatalf("directory entries = %d, want %d source and unrelated files", len(entries), len(paths)+1)
	}
	assertNoOpenFilesInDirectory(t, directory)
}

func TestMergeRunsProducesSameBytesAcrossFanInAndWorkers(t *testing.T) {
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
	want := encodeMergeTestRun(t, runHeader{documentCount: runCount}, []mergeTestTerm{
		{term: "shared", postings: postings},
	})

	tests := []struct {
		name        string
		fanIn       int
		workers     int
		groupCounts []int
	}{
		{name: "fan-in 2", fanIn: 2, workers: 1, groupCounts: []int{5, 3, 2, 1}},
		{name: "fan-in 3", fanIn: 3, workers: 1, groupCounts: []int{4, 2, 1}},
		{name: "one pass", fanIn: 10, workers: 1, groupCounts: []int{1}},
		{name: "two workers", fanIn: 2, workers: 2, groupCounts: []int{5, 3, 2, 1}},
		{name: "four workers", fanIn: 2, workers: 4, groupCounts: []int{5, 3, 2, 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := writeMergeTestRuns(t, t.TempDir(), runs)
			path, stats, err := mergeRuns(context.Background(), t.TempDir(), paths, test.fanIn, test.workers)
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

func TestMergeRunsMatchesInMemoryIndex(t *testing.T) {
	input, err := os.ReadFile("../index/testdata/corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	want, err := index.Build(corpus.NewTSVReader(bytes.NewReader(input)))
	if err != nil {
		t.Fatal(err)
	}

	result, err := build(
		context.Background(),
		corpus.NewTSVReader(bytes.NewReader(input)),
		segmentBufferBytes,
		t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.runPaths) < 3 {
		t.Fatalf("run count = %d, want at least 3 for a multipass merge", len(result.runPaths))
	}

	path, _, err := mergeRuns(context.Background(), result.directory, result.runPaths, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	result.runPaths = []string{path}
	if got := readBuildIndex(t, result); !reflect.DeepEqual(got, want) {
		t.Fatalf("logical index = %#v, want %#v", got, want)
	}
}

func TestMergeRunsPreservesCurrentPassSourcesOnLaterFailure(t *testing.T) {
	directory := t.TempDir()
	runs := make([][]byte, 3)
	for i, documentID := range []index.DocumentID{0, 1, 3} {
		runs[i] = encodeMergeTestRun(t, runHeader{
			firstDocumentID: documentID,
			documentCount:   1,
		}, nil)
	}
	paths := writeMergeTestRuns(t, directory, runs)

	if _, _, err := mergeRuns(context.Background(), directory, paths, 2, 1); err == nil {
		t.Fatal("mergeRuns() error = nil")
	}
	if _, err := os.Stat(paths[2]); err != nil {
		t.Fatalf("stat preserved file %q: %v", paths[2], err)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("directory entries = %d, want 2", len(entries))
	}

	var successorPath string
	for _, entry := range entries {
		if entry.Name() != filepath.Base(paths[2]) {
			successorPath = filepath.Join(directory, entry.Name())
		}
	}
	got, err := os.ReadFile(successorPath)
	if err != nil {
		t.Fatal(err)
	}
	want := encodeMergeTestRun(t, runHeader{documentCount: 2}, nil)
	if !bytes.Equal(got, want) {
		t.Fatal("preserved pass source does not match expected bytes")
	}
}

func TestMergeRunsCreatesCanonicalEmptyRun(t *testing.T) {
	path, stats, err := mergeRuns(context.Background(), t.TempDir(), nil, 2, 1)
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

	gotPath, stats, err := mergeRuns(context.Background(), t.TempDir(), []string{path}, 2, 1)
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
	if _, _, err := mergeRuns(context.Background(), t.TempDir(), []string{corruptPath}, 2, 1); err == nil {
		t.Fatal("mergeRuns() error = nil for corrupt sole run")
	}
}

func TestValidateRunStopsDuringHotTermWhenCanceled(t *testing.T) {
	data := encodeHotTermRun(t, 0, 1<<16)
	ctx, cancel := context.WithCancel(context.Background())
	input := &cancelingReader{
		Reader:           bytes.NewReader(data),
		cancel:           cancel,
		readsUntilCancel: 2,
	}

	if err := validateRun(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("validateRun() error = %v, want %v", err, context.Canceled)
	}
	if input.readBytes > 2*runBufferBytes {
		t.Fatalf("bytes read after cancellation = %d, want at most %d", input.readBytes, 2*runBufferBytes)
	}
}

func TestMergeRunsRejectsInvalidConfigurationBeforeCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	for _, config := range []struct {
		fanIn   int
		workers int
	}{
		{fanIn: 1, workers: 1},
		{fanIn: 2, workers: 0},
	} {
		if _, _, err := mergeRuns(context.Background(), directory, nil, config.fanIn, config.workers); err == nil {
			t.Fatalf("mergeRuns(fanIn=%d, workers=%d) error = nil", config.fanIn, config.workers)
		}
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("output count = %d, want 0", len(entries))
	}
}

func TestMergeRunsRejectsCanceledContextBeforeCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := mergeRuns(ctx, directory, nil, 2, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("mergeRuns() error = %v, want %v", err, context.Canceled)
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

type cancelingReader struct {
	io.Reader
	cancel           context.CancelFunc
	readsUntilCancel int
	readBytes        int
}

func assertNoOpenFilesInDirectory(t *testing.T, directory string) {
	t.Helper()

	descriptors, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	prefix := directory + string(os.PathSeparator)
	var open []string
	for _, descriptor := range descriptors {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", descriptor.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.TrimSuffix(target, " (deleted)"), prefix) {
			open = append(open, target)
		}
	}
	if len(open) != 0 {
		t.Fatalf("open files in merge directory = %v, want none", open)
	}
}

func (r *cancelingReader) Read(buffer []byte) (int, error) {
	n, err := r.Reader.Read(buffer)
	r.readBytes += n
	r.readsUntilCancel--
	if r.readsUntilCancel == 0 {
		r.cancel()
	}
	return n, err
}
