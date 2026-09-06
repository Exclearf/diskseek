package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/Exclearf/diskseek/internal/segment"
)

func TestSearchUsesSelectedDataset(t *testing.T) {
	wiki := openTestIndex(t, strings.Join([]string{
		"shared\tcomputer science",
		"wiki-1\tcomputer",
		"wiki-2\tcomputer",
		"wiki-3\tcomputer",
		"wiki-4\tcomputer",
		"wiki-5\tcomputer",
		"",
	}, "\n"))
	bee := openTestIndex(t, "shared\thoney bee\nbee\tbee bee\n")
	handler := New(map[string]*indexfile.Index{
		"wiki": wiki,
		"bee":  bee,
	})

	for _, test := range []struct {
		dataset   string
		query     string
		index     *indexfile.Index
		wantCount int
	}{
		{dataset: "wiki", query: "computer", index: wiki, wantCount: 5},
		{dataset: "bee", query: "bee", index: bee, wantCount: 2},
	} {
		t.Run(test.dataset, func(t *testing.T) {
			body := `{"dataset":"` + test.dataset + `","query":"` + test.query + `"}`
			request := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}

			var got searchResponse
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			want, err := search.Search(t.Context(), test.index, test.query, resultLimit)
			if err != nil {
				t.Fatalf("direct search: %v", err)
			}
			if len(got.Results) != test.wantCount || len(got.Results) != len(want) {
				t.Fatalf("results = %d, want %d", len(got.Results), test.wantCount)
			}
			for position := range want {
				if got.Results[position].ExternalID != want[position].ExternalID ||
					math.Float64bits(got.Results[position].Score) != math.Float64bits(want[position].Score) {
					t.Fatalf("result %d = %+v, want %+v", position, got.Results[position], want[position])
				}
			}
		})
	}
}

func TestSearchRejectsInvalidRequest(t *testing.T) {
	handler := New(map[string]*indexfile.Index{"wiki": openTestIndex(t, "doc\tgo\n")})
	for _, body := range []string{
		`{"dataset":"other","query":"go"}`,
		`{"dataset":"wiki","query":"go"}` + strings.Repeat(" ", maxRequestBodyBytes),
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
		}
	}
}

func openTestIndex(t *testing.T, input string) *indexfile.Index {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "index")
	_, err := segment.BuildIndex(
		t.Context(),
		corpus.NewTSVReader(strings.NewReader(input)),
		destination,
		segment.BuildOptions{
			FlushTarget:  1 << 20,
			MergeFanIn:   2,
			MergeWorkers: 1,
			Codec:        indexfile.PostingsCodecVByte,
		},
	)
	if err != nil {
		t.Fatalf("build test index: %v", err)
	}
	idx, err := indexfile.Open(destination)
	if err != nil {
		t.Fatalf("open test index: %v", err)
	}
	t.Cleanup(func() {
		if err := idx.Close(); err != nil {
			t.Errorf("close test index: %v", err)
		}
	})
	return idx
}
