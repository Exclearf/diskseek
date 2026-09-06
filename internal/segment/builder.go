package segment

import (
	"context"
	"errors"
	"io"

	"github.com/Exclearf/diskseek/internal/analyzer"
	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

type buildStats struct {
	documentCount      uint64
	documentsWithTerms uint64
	totalTokenCount    uint64
	postingCount       uint64
	maxAccountedBytes  uint64
}

func buildRuns(
	ctx context.Context,
	records *corpus.TSVReader,
	flushTarget uint64,
	documentOutput io.WriteCloser,
	createOutput func() (io.WriteCloser, error),
) (stats buildStats, err error) {
	documents, err := newDocumentWriter(documentOutput)
	if err != nil {
		return buildStats{}, err
	}
	defer func() {
		err = errors.Join(err, documents.close())
	}()

	segment := newSegmentState(0)
	flush := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		output, err := createOutput()
		if err != nil {
			return err
		}
		if err := segment.writeRun(output); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		segment = newSegmentState(index.DocumentID(stats.documentCount))
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return buildStats{}, err
		}
		record, err := records.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return buildStats{}, err
		}
		if stats.documentCount == documentIDLimit {
			return buildStats{}, errors.New("document count exceeds supported limit")
		}

		tokens, err := analyzer.Analyze(record.Text)
		if err != nil {
			return buildStats{}, err
		}
		if err := ctx.Err(); err != nil {
			return buildStats{}, err
		}
		documentLength := uint32(len(tokens))
		accountedBytes, postingCount := segment.addDocument(tokens)
		stats.maxAccountedBytes = max(stats.maxAccountedBytes, accountedBytes)
		if err := documents.write(index.DocumentMeta{
			ExternalID: record.ExternalID,
			Length:     documentLength,
		}); err != nil {
			return buildStats{}, err
		}
		stats.documentCount++
		if documentLength != 0 {
			stats.documentsWithTerms++
		}
		stats.totalTokenCount += uint64(documentLength)
		stats.postingCount += postingCount

		if accountedBytes >= flushTarget {
			if err := flush(); err != nil {
				return buildStats{}, err
			}
		}
	}

	if segment.documentCount != 0 {
		if err := flush(); err != nil {
			return buildStats{}, err
		}
	}
	return stats, nil
}
