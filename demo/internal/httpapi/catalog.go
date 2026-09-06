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
	Title     string `json:"title"`
	Preview   string `json:"preview"`
	SourceURL string `json:"source_url"`
}

type catalogRecord struct {
	ExternalID string `json:"external_id"`
	Document
}

func LoadCatalog(input io.Reader) (map[string]Document, error) {
	documents := make(map[string]Document)
	scanner := bufio.NewScanner(input)
	for line := 1; scanner.Scan(); line++ {
		var record catalogRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode catalog line %d: %w", line, err)
		}
		if record.ExternalID == "" || record.Title == "" {
			return nil, fmt.Errorf("catalog line %d requires an external ID and title", line)
		}
		if _, exists := documents[record.ExternalID]; exists {
			return nil, fmt.Errorf("duplicate catalog external ID %q", record.ExternalID)
		}
		if record.SourceURL != "" {
			source, err := url.Parse(record.SourceURL)
			if err != nil || source.Scheme != "https" || source.Host == "" {
				return nil, fmt.Errorf("catalog line %d has an invalid source URL", line)
			}
		}
		documents[record.ExternalID] = record.Document
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
