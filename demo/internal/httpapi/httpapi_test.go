package httpapi

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/Exclearf/diskseek/internal/segment"
)

func TestSearch(t *testing.T) {
	wiki := openTestDataset(t, strings.Join([]string{
		"wiki\tcomputer science",
		"wiki-1\tcomputer",
		"wiki-2\tcomputer",
		"wiki-3\tcomputer",
		"wiki-4\tcomputer",
		"wiki-5\tcomputer",
		"",
	}, "\n"), strings.Join([]string{
		`{"external_id":"wiki","title":"Computer science","preview":"Computer science","source_url":"https://example.com/wiki"}`,
		`{"external_id":"wiki-1","title":"Computer 1","preview":"Preview 1","source_url":"https://example.com/1"}`,
		`{"external_id":"wiki-2","title":"Computer 2","preview":"Preview 2","source_url":"https://example.com/2"}`,
		`{"external_id":"wiki-3","title":"Computer 3","preview":"Preview 3","source_url":"https://example.com/3"}`,
		`{"external_id":"wiki-4","title":"Computer 4","preview":"Preview 4","source_url":"https://example.com/4"}`,
		`{"external_id":"wiki-5","title":"Computer 5","preview":"Preview 5","source_url":"https://example.com/5"}`,
	}, "\n"))
	handler := New(wiki)

	for _, test := range []struct {
		name      string
		query     string
		limit     int
		index     *indexfile.Index
		catalog   map[string]Document
		wantCount int
	}{
		{name: "default limit", query: "computer", index: wiki.Index, catalog: wiki.Catalog, wantCount: 5},
		{name: "requested limit", query: "computer", limit: 2, index: wiki.Index, catalog: wiki.Catalog, wantCount: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(searchRequest{Query: test.query, Limit: test.limit})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}

			var got searchResponse
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.SearchMS <= 0 {
				t.Fatalf("search_ms = %v, want a positive duration", got.SearchMS)
			}
			limit := test.limit
			if limit == 0 {
				limit = defaultResultLimit
			}
			want, err := search.Search(t.Context(), test.index, test.query, limit)
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
	dataset := openTestDataset(t, "doc\tgo\n", `{"external_id":"doc","title":"Document"}`)
	handler := New(dataset)
	for _, body := range []string{
		`{"query":"go","limit":21}`,
		`{"query":"go"}` + strings.Repeat(" ", maxRequestBodyBytes),
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
	handler := New(openTestDataset(t, "doc\tgo\n", ""))
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/search",
		strings.NewReader(`{"query":"go"}`),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func openTestDataset(t *testing.T, corpusInput, catalogInput string) Dataset {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "index")
	_, err := segment.BuildIndex(
		t.Context(),
		corpus.NewTSVReader(strings.NewReader(corpusInput)),
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
	if err := os.WriteFile(filepath.Join(destination, catalogFileName), []byte(catalogInput), 0o600); err != nil {
		t.Fatalf("write test catalog: %v", err)
	}
	dataset, err := OpenDataset(destination)
	if err != nil {
		t.Fatalf("open test dataset: %v", err)
	}
	t.Cleanup(func() {
		if err := dataset.Index.Close(); err != nil {
			t.Errorf("close test index: %v", err)
		}
	})
	return dataset
}
