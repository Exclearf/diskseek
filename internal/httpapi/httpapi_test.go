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
	wikiCatalog := loadTestCatalog(t, strings.Join([]string{
		`{"external_id":"shared","title":"Shared wiki page","preview":"Computer science","source_url":"https://example.com/shared"}`,
		`{"external_id":"wiki-1","title":"Computer 1","preview":"Preview 1","source_url":"https://example.com/1"}`,
		`{"external_id":"wiki-2","title":"Computer 2","preview":"Preview 2","source_url":"https://example.com/2"}`,
		`{"external_id":"wiki-3","title":"Computer 3","preview":"Preview 3","source_url":"https://example.com/3"}`,
		`{"external_id":"wiki-4","title":"Computer 4","preview":"Preview 4","source_url":"https://example.com/4"}`,
		`{"external_id":"wiki-5","title":"Computer 5","preview":"Preview 5","source_url":"https://example.com/5"}`,
	}, "\n"))
	beeCatalog := loadTestCatalog(t, strings.Join([]string{
		`{"external_id":"shared","title":"Shared bee passage","preview":"Honey bee","source_url":"https://example.com/bee/shared"}`,
		`{"external_id":"bee","title":"Bee passage","preview":"Bee bee","source_url":"https://example.com/bee"}`,
	}, "\n"))
	handler := New(map[string]Dataset{
		"wiki": {Index: wiki, Catalog: wikiCatalog},
		"bee":  {Index: bee, Catalog: beeCatalog},
	})

	for _, test := range []struct {
		dataset   string
		query     string
		index     *indexfile.Index
		catalog   map[string]Document
		wantCount int
	}{
		{dataset: "wiki", query: "computer", index: wiki, catalog: wikiCatalog, wantCount: 5},
		{dataset: "bee", query: "bee", index: bee, catalog: beeCatalog, wantCount: 2},
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
				if got.Results[position].Document != test.catalog[want[position].ExternalID] {
					t.Fatalf("result %d metadata = %+v, want %+v", position, got.Results[position].Document, test.catalog[want[position].ExternalID])
				}
			}
		})
	}
}

func TestSearchRejectsInvalidRequest(t *testing.T) {
	handler := New(map[string]Dataset{"wiki": {Index: openTestIndex(t, "doc\tgo\n")}})
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

func TestLoadCatalogRejectsInvalidMetadata(t *testing.T) {
	for _, input := range []string{
		"{\"external_id\":\"doc\",\"title\":\"One\"}\n{\"external_id\":\"doc\",\"title\":\"Two\"}\n",
		"{\"external_id\":\"doc\",\"title\":\"\"}\n",
		"{\"external_id\":\"doc\",\"title\":\"Title\",\"source_url\":\"http://example.com\"}\n",
	} {
		if _, err := LoadCatalog(strings.NewReader(input)); err == nil {
			t.Fatalf("LoadCatalog(%q) error = nil", input)
		}
	}
}

func TestSearchFailsWithoutResultMetadata(t *testing.T) {
	handler := New(map[string]Dataset{
		"wiki": {Index: openTestIndex(t, "doc\tgo\n")},
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/search",
		strings.NewReader(`{"dataset":"wiki","query":"go"}`),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func loadTestCatalog(t *testing.T, input string) map[string]Document {
	t.Helper()
	catalog, err := LoadCatalog(strings.NewReader(input))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
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
