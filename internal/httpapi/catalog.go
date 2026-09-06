package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

const catalogFileName = "catalog.jsonl"

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

func OpenDataset(directory string) (Dataset, error) {
	idx, err := indexfile.Open(directory)
	if err != nil {
		return Dataset{}, err
	}

	input, err := os.Open(filepath.Join(directory, catalogFileName))
	if err != nil {
		return Dataset{}, errors.Join(err, idx.Close())
	}
	catalog, loadErr := LoadCatalog(input)
	if err := errors.Join(loadErr, input.Close()); err != nil {
		return Dataset{}, errors.Join(err, idx.Close())
	}
	return Dataset{Index: idx, Catalog: catalog}, nil
}
