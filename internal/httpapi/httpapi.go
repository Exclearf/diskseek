package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Exclearf/diskseek/internal/indexfile"
	"github.com/Exclearf/diskseek/internal/search"
	"github.com/go-chi/chi/v5"
)

const (
	maxRequestBodyBytes = 8 << 10
	resultLimit         = 5
)

type searchRequest struct {
	Dataset string `json:"dataset"`
	Query   string `json:"query"`
}

type searchResult struct {
	ExternalID string  `json:"external_id"`
	Score      float64 `json:"score"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

func New(datasets map[string]*indexfile.Index) http.Handler {
	router := chi.NewRouter()
	router.Post("/v1/search", func(response http.ResponseWriter, request *http.Request) {
		var input searchRequest
		body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, maxRequestBodyBytes))
		if err != nil || json.Unmarshal(body, &input) != nil {
			http.Error(response, "invalid request", http.StatusBadRequest)
			return
		}

		idx := datasets[input.Dataset]
		if idx == nil {
			http.Error(response, "unknown dataset", http.StatusBadRequest)
			return
		}

		results, err := search.Search(request.Context(), idx, input.Query, resultLimit)
		if err != nil {
			http.Error(response, "search failed", http.StatusInternalServerError)
			return
		}

		output := make([]searchResult, len(results))
		for position, result := range results {
			output[position] = searchResult{
				ExternalID: result.ExternalID,
				Score:      result.Score,
			}
		}

		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(searchResponse{Results: output})
	})
	return router
}
