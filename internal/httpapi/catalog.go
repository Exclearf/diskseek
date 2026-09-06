package httpapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
)

type Document struct {
	ExternalID string `json:"external_id"`
	Title      string `json:"title"`
	Preview    string `json:"preview"`
	SourceURL  string `json:"source_url"`
}

func LoadCatalog(input io.Reader) (map[string]Document, error) {
	documents := make(map[string]Document)
	scanner := bufio.NewScanner(input)
	for line := 1; scanner.Scan(); line++ {
		var document Document
		if err := json.Unmarshal(scanner.Bytes(), &document); err != nil {
			return nil, fmt.Errorf("decode catalog line %d: %w", line, err)
		}
		if document.ExternalID == "" || document.Title == "" {
			return nil, fmt.Errorf("catalog line %d requires an external ID and title", line)
		}
		if _, exists := documents[document.ExternalID]; exists {
			return nil, fmt.Errorf("duplicate catalog external ID %q", document.ExternalID)
		}
		if document.SourceURL != "" {
			source, err := url.Parse(document.SourceURL)
			if err != nil || source.Scheme != "https" || source.Host == "" {
				return nil, fmt.Errorf("catalog line %d has an invalid source URL", line)
			}
		}
		documents[document.ExternalID] = document
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	return documents, nil
}
