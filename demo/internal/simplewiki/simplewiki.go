package simplewiki

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/corpus"
)

const (
	maxDumpRecordBytes = 64 << 20
	maxPreviewRunes    = 320
)

type Stats struct {
	Documents         uint64
	SkippedNamespaces uint64
	SkippedEmpty      uint64
	SkippedOversized  uint64
}

type dumpAction struct {
	Index struct {
		ID uint64 `json:"_id"`
	} `json:"index"`
}

type dumpArticle struct {
	PageID      uint64 `json:"page_id"`
	Namespace   int    `json:"namespace"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	OpeningText string `json:"opening_text"`
}

type catalogRecord struct {
	ExternalID string `json:"external_id"`
	Title      string `json:"title"`
	Preview    string `json:"preview"`
	SourceURL  string `json:"source_url"`
}

func Convert(ctx context.Context, input io.Reader, corpusOutput, catalogOutput io.Writer) (Stats, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(nil, maxDumpRecordBytes)
	corpusWriter := bufio.NewWriter(corpusOutput)
	catalogWriter := bufio.NewWriter(catalogOutput)
	catalogEncoder := json.NewEncoder(catalogWriter)

	var stats Stats
	for record := uint64(1); scanner.Scan(); record++ {
		if err := ctx.Err(); err != nil {
			return Stats{}, err
		}
		var action dumpAction
		if err := json.Unmarshal(scanner.Bytes(), &action); err != nil {
			return Stats{}, fmt.Errorf("decode dump action %d: %w", record, err)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return Stats{}, fmt.Errorf("read dump article %d: %w", record, err)
			}
			return Stats{}, errors.New("dump action has no article")
		}

		var article dumpArticle
		if err := json.Unmarshal(scanner.Bytes(), &article); err != nil {
			return Stats{}, fmt.Errorf("decode dump article %d: %w", record, err)
		}
		if action.Index.ID != article.PageID {
			return Stats{}, fmt.Errorf("dump record %d has mismatched page IDs", record)
		}
		if article.Namespace != 0 {
			stats.SkippedNamespaces++
			continue
		}

		title := singleLine(article.Title)
		text := singleLine(article.Text)
		if title == "" || text == "" {
			stats.SkippedEmpty++
			continue
		}

		externalID := strconv.FormatUint(article.PageID, 10)
		searchText := title + " " + text
		if len(externalID)+1+len(searchText) > corpus.MaxRecordBytes {
			stats.SkippedOversized++
			continue
		}
		if _, err := fmt.Fprintf(corpusWriter, "%s\t%s\n", externalID, searchText); err != nil {
			return Stats{}, fmt.Errorf("write corpus record: %w", err)
		}

		preview := singleLine(article.OpeningText)
		if preview == "" {
			preview = text
		}
		preview = truncate(preview, maxPreviewRunes)
		source := url.URL{
			Scheme: "https",
			Host:   "simple.wikipedia.org",
			Path:   "/wiki/" + strings.ReplaceAll(article.Title, " ", "_"),
		}
		if err := catalogEncoder.Encode(catalogRecord{
			ExternalID: externalID,
			Title:      title,
			Preview:    preview,
			SourceURL:  source.String(),
		}); err != nil {
			return Stats{}, fmt.Errorf("write catalog record: %w", err)
		}
		stats.Documents++
	}
	if err := scanner.Err(); err != nil {
		return Stats{}, fmt.Errorf("read dump: %w", err)
	}
	if err := errors.Join(corpusWriter.Flush(), catalogWriter.Flush()); err != nil {
		return Stats{}, fmt.Errorf("flush output: %w", err)
	}
	return stats, nil
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
