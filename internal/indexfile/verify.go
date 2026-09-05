package indexfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Verify validates every file and posting in an index directory.
func Verify(ctx context.Context, directory string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	metadata, err := verifyIndexStructure(directory)
	if err != nil {
		return err
	}

	var inputs []*os.File
	defer func() {
		for position := len(inputs) - 1; position >= 0; position-- {
			err = errors.Join(err, inputs[position].Close())
		}
	}()
	open := func(name string) (*os.File, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
		return input, nil
	}

	lengthInput, err := open(DocumentLengthsFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", DocumentLengthsFileName, err)
	}
	offsetInput, err := open(DocumentOffsetsFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", DocumentOffsetsFileName, err)
	}
	dataInput, err := open(DocumentDataFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", DocumentDataFileName, err)
	}
	termInput, err := open(TermsFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", TermsFileName, err)
	}
	postingInput, err := open(PostingsFileName)
	if err != nil {
		return fmt.Errorf("open %s: %w", PostingsFileName, err)
	}

	lengths, err := readDocumentLengths(
		contextReader{ctx, lengthInput},
		int64(metadata.Documents.Lengths.Length),
	)
	if err != nil {
		return fmt.Errorf("verify %s: %w", DocumentLengthsFileName, err)
	}
	documentCount := uint64(len(lengths.values))
	if err := verifyExternalIDFiles(
		contextReader{ctx, offsetInput},
		int64(metadata.Documents.Offsets.Length),
		contextReader{ctx, dataInput},
		int64(metadata.Documents.Data.Length),
		documentCount,
	); err != nil {
		return fmt.Errorf("verify external document IDs: %w", err)
	}

	postingsBodyBytes := metadata.Terms.Postings.Length - minimumFileBytes
	terms, err := readTermFile(
		contextReader{ctx, termInput},
		int64(metadata.Terms.Terms.Length),
		postingsBodyBytes,
		lengths.documentsWithTerms,
	)
	if err != nil {
		return fmt.Errorf("verify %s: %w", TermsFileName, err)
	}
	if err := verifyPostingsFile(
		ctx,
		contextReader{ctx, postingInput},
		int64(metadata.Terms.Postings.Length),
		metadata.Terms.Codec,
		terms,
		lengths.values,
	); err != nil {
		return fmt.Errorf("verify %s: %w", PostingsFileName, err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
