package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/go-chi/chi/v5"
)

const (
	maxRequestBodyBytes = 8 << 10
	defaultResultLimit  = 5
	maxResultLimit      = 20
)

type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type searchResult struct {
	ExternalID string `json:"external_id"`
	Document
	Score float64 `json:"score"`
}

type searchResponse struct {
	Results  []searchResult `json:"results"`
	SearchMS float64        `json:"search_ms"`
}

type Dataset struct {
	Index   *indexfile.Index
	Catalog map[string]Document
}

func New(dataset Dataset) http.Handler {
	router := chi.NewRouter()
	router.Post("/v1/search", func(response http.ResponseWriter, request *http.Request) {
		var input searchRequest
		body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, maxRequestBodyBytes))
		if err != nil || json.Unmarshal(body, &input) != nil {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}
		if input.Limit == 0 {
			input.Limit = defaultResultLimit
		}
		if input.Limit < 1 || input.Limit > maxResultLimit {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}

		started := time.Now()
		results, err := search.Search(request.Context(), dataset.Index, input.Query, input.Limit)
		searchDuration := time.Since(started)
		if err != nil {
			http.Error(response, "search failed", http.StatusInternalServerError)
			return
		}

		output := make([]searchResult, len(results))
		for position, result := range results {
			document, found := dataset.Catalog[result.ExternalID]
			if !found {
				http.Error(response, "search failed", http.StatusInternalServerError)
				return
			}
			output[position] = searchResult{
				ExternalID: result.ExternalID,
				Document:   document,
				Score:      result.Score,
			}
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(searchResponse{
			Results:  output,
			SearchMS: float64(searchDuration) / float64(time.Millisecond),
		})
	})
	return router
}
