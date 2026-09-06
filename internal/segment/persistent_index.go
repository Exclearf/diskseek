package segment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

func writeIndex(
	ctx context.Context,
	destination, runPath, documentsPath string,
	codec indexfile.PostingsCodec,
) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o755); err != nil {
		return fmt.Errorf("create index directory: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(destination))
		}
	}()

	var openFiles []*os.File
	closeOpenFiles := func() error {
		var closeErr error
		for _, file := range openFiles {
			closeErr = errors.Join(closeErr, file.Close())
		}
		return closeErr
	}
	defer func() {
		err = errors.Join(err, closeOpenFiles())
	}()

	run, err := os.Open(runPath)
	if err != nil {
		return fmt.Errorf("open merged run: %w", err)
	}
	openFiles = append(openFiles, run)
	documents, err := os.Open(documentsPath)
	if err != nil {
		return fmt.Errorf("open document sidecar: %w", err)
	}
	openFiles = append(openFiles, documents)

	terms, err := os.Create(filepath.Join(destination, indexfile.TermsFileName))
	if err != nil {
		return fmt.Errorf("create terms file: %w", err)
	}
	openFiles = append(openFiles, terms)
	postings, err := os.Create(filepath.Join(destination, indexfile.PostingsFileName))
	if err != nil {
		return fmt.Errorf("create postings file: %w", err)
	}
	openFiles = append(openFiles, postings)
	documentLengths, err := os.Create(filepath.Join(destination, indexfile.DocumentLengthsFileName))
	if err != nil {
		return fmt.Errorf("create document lengths file: %w", err)
	}
	openFiles = append(openFiles, documentLengths)
	documentOffsets, err := os.Create(filepath.Join(destination, indexfile.DocumentOffsetsFileName))
	if err != nil {
		return fmt.Errorf("create document offsets file: %w", err)
	}
	openFiles = append(openFiles, documentOffsets)
	documentData, err := os.Create(filepath.Join(destination, indexfile.DocumentDataFileName))
	if err != nil {
		return fmt.Errorf("create document data file: %w", err)
	}
	openFiles = append(openFiles, documentData)

	termMetadata, err := writePersistentTermFiles(ctx, run, terms, postings, codec)
	if err != nil {
		return fmt.Errorf("write term files: %w", err)
	}
	documentMetadata, err := writePersistentDocumentFiles(
		ctx,
		documents,
		documentLengths,
		documentOffsets,
		documentData,
	)
	if err != nil {
		return fmt.Errorf("write document files: %w", err)
	}

	closeErr := closeOpenFiles()
	openFiles = nil
	if closeErr != nil {
		return fmt.Errorf("close index data files: %w", closeErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	metadata, err := os.Create(filepath.Join(destination, indexfile.MetadataFileName))
	if err != nil {
		return fmt.Errorf("create metadata file: %w", err)
	}
	openFiles = append(openFiles, metadata)
	if err := indexfile.WriteMetadataFile(metadata, termMetadata, documentMetadata); err != nil {
		return fmt.Errorf("write metadata file: %w", err)
	}
	closeErr = metadata.Close()
	openFiles = nil
	if closeErr != nil {
		return fmt.Errorf("close metadata file: %w", closeErr)
	}
	return nil
}

func writePersistentDocumentFiles(
	ctx context.Context,
	sidecar io.Reader,
	lengthOutput io.Writer,
	offsetOutput io.Writer,
	dataOutput io.Writer,
) (indexfile.DocumentFilesMetadata, error) {
	documents, err := newDocumentReader(contextReader{ctx, sidecar})
	if err != nil {
		return indexfile.DocumentFilesMetadata{}, err
	}
	return indexfile.WriteDocumentFiles(lengthOutput, offsetOutput, dataOutput, documents.next)
}

func writePersistentTermFiles(
	ctx context.Context,
	run io.Reader,
	termOutput io.Writer,
	postingOutput io.Writer,
	codec indexfile.PostingsCodec,
) (indexfile.TermFilesMetadata, error) {
	terms, err := newRunReader(contextReader{ctx, run})
	if err != nil {
		return indexfile.TermFilesMetadata{}, err
	}
	return indexfile.WriteTermFiles(termOutput, postingOutput, codec, terms.nextTerm, terms.nextPosting)
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
