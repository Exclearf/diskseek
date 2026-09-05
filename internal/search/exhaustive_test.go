package search

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/Exclearf/diskseek/internal/analyzer"
	"github.com/Exclearf/diskseek/internal/index"
	"github.com/Exclearf/diskseek/internal/indexfile"
)

func TestExhaustiveDAATTraversesPostingUnion(t *testing.T) {
	idx := openDiskTestIndex(t, filepath.Join("..", "indexfile", "testdata", "golden-v1", "vbyte"))

	got, stats, err := searchDAAT(idx, "search go", 10)
	if err != nil {
		t.Fatal(err)
	}

	goIDF := bm25IDF(2, 2)
	searchIDF := bm25IDF(2, 1)
	documentZeroScore := bm25TermScore(goIDF, 1, 2, 2.5)
	documentZeroScore += bm25TermScore(searchIDF, 1, 2, 2.5)
	want := []result{
		{DocumentID: 0, ExternalID: "a", Score: documentZeroScore},
		{DocumentID: 1, ExternalID: "b", Score: bm25TermScore(goIDF, 3, 3, 2.5)},
	}

	if len(got) != len(want) {
		t.Fatalf("searchDAAT() returned %d results, want %d", len(got), len(want))
	}
	for position := range want {
		if got[position].DocumentID != want[position].DocumentID ||
			got[position].ExternalID != want[position].ExternalID ||
			math.Float64bits(got[position].Score) != math.Float64bits(want[position].Score) {
			t.Fatalf("result %d = %+v, want %+v", position, got[position], want[position])
		}
	}
	if wantStats := (daatStats{
		PostingsDecoded:  3,
		NextCalls:        3,
		CandidatesScored: 2,
		BytesRequested:   22,
	}); stats != wantStats {
		t.Fatalf("stats = %+v, want %+v", stats, wantStats)
	}
}

func TestExhaustiveDAATEmptyResults(t *testing.T) {
	idx := openDiskTestIndex(t, filepath.Join("..", "indexfile", "testdata", "golden-v1", "vbyte"))
	for _, test := range []struct {
		name  string
		query string
		k     int
	}{
		{name: "zero k", query: "go"},
		{name: "empty query", query: "---", k: 10},
		{name: "unknown term", query: "missing", k: 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			results, _, err := searchDAAT(idx, test.query, test.k)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 0 {
				t.Fatalf("searchDAAT() returned %d results, want 0", len(results))
			}
		})
	}
}

func TestSearchDAATValidatesQueryWithZeroK(t *testing.T) {
	idx := openDiskTestIndex(t, filepath.Join("..", "indexfile", "testdata", "golden-v1", "vbyte"))
	results, stats, err := searchDAAT(idx, string([]byte{0xff}), 0)
	if !errors.Is(err, analyzer.ErrInvalidUTF8) || results != nil || stats != (daatStats{}) {
		t.Fatalf("searchDAAT() = (%v, %+v, %v), want (nil, zero stats, %v)", results, stats, err, analyzer.ErrInvalidUTF8)
	}
}

func TestExhaustiveDAATDiscardsPartialResults(t *testing.T) {
	t.Run("open first cursor", func(t *testing.T) {
		directory := copyDiskTestIndex(t)
		idx := openDiskTestIndex(t, directory)
		if err := os.Truncate(filepath.Join(directory, indexfile.PostingsFileName), 0); err != nil {
			t.Fatal(err)
		}

		results, _, err := searchDAAT(idx, "go", 10)
		if err == nil || results != nil {
			t.Fatalf("searchDAAT() = (%v, %v), want (nil, error)", results, err)
		}
	})

	t.Run("advance to later block", func(t *testing.T) {
		directory := writeLongDiskTestIndex(t)
		idx := openDiskTestIndex(t, directory)
		plan, err := buildDiskQueryPlan(idx, "term")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(filepath.Join(directory, indexfile.PostingsFileName), 0); err != nil {
			t.Fatal(err)
		}

		results, _, err := executeDAAT(idx, plan, 10)
		if err == nil || results != nil {
			t.Fatalf("executeDAAT() = (%v, %v), want (nil, error)", results, err)
		}
	})

	t.Run("resolve external ID", func(t *testing.T) {
		directory := copyDiskTestIndex(t)
		idx := openDiskTestIndex(t, directory)
		if err := os.Truncate(filepath.Join(directory, indexfile.DocumentDataFileName), 0); err != nil {
			t.Fatal(err)
		}

		results, _, err := searchDAAT(idx, "go", 10)
		if err == nil || results != nil {
			t.Fatalf("searchDAAT() = (%v, %v), want (nil, error)", results, err)
		}
	})
}

func openDiskTestIndex(t *testing.T, directory string) *indexfile.Index {
	t.Helper()
	idx, err := indexfile.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := idx.Close(); err != nil {
			t.Error(err)
		}
	})
	return idx
}

func copyDiskTestIndex(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	source := filepath.Join("..", "indexfile", "testdata", "golden-v1", "vbyte")
	if err := os.CopyFS(directory, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return directory
}

func writeLongDiskTestIndex(t *testing.T) string {
	t.Helper()
	const documentCount = 129

	var terms, postings bytes.Buffer
	termRead := false
	postingID := 0
	termMetadata, err := indexfile.WriteTermFiles(
		&terms,
		&postings,
		indexfile.PostingsCodecVByte,
		func() (string, uint64, error) {
			if termRead {
				return "", 0, io.EOF
			}
			termRead = true
			return "term", documentCount, nil
		},
		func() (index.Posting, error) {
			posting := index.Posting{DocumentID: index.DocumentID(postingID), Frequency: 1}
			postingID++
			return posting, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var lengths, offsets, data bytes.Buffer
	documentID := 0
	documentMetadata, err := indexfile.WriteDocumentFiles(
		&lengths,
		&offsets,
		&data,
		func() (index.DocumentMeta, error) {
			if documentID == documentCount {
				return index.DocumentMeta{}, io.EOF
			}
			documentID++
			return index.DocumentMeta{ExternalID: "d", Length: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var metadata bytes.Buffer
	if err := indexfile.WriteMetadataFile(&metadata, termMetadata, documentMetadata); err != nil {
		t.Fatal(err)
	}

	directory := t.TempDir()
	files := map[string][]byte{
		indexfile.MetadataFileName:        metadata.Bytes(),
		indexfile.TermsFileName:           terms.Bytes(),
		indexfile.PostingsFileName:        postings.Bytes(),
		indexfile.DocumentLengthsFileName: lengths.Bytes(),
		indexfile.DocumentOffsetsFileName: offsets.Bytes(),
		indexfile.DocumentDataFileName:    data.Bytes(),
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}
