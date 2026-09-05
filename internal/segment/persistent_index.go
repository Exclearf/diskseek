package segment

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

func writeIndex(destination, runPath, documentsPath string) (err error) {
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

	terms, err := os.Create(filepath.Join(destination, "terms.bin"))
	if err != nil {
		return fmt.Errorf("create terms file: %w", err)
	}
	openFiles = append(openFiles, terms)
	postings, err := os.Create(filepath.Join(destination, "postings.bin"))
	if err != nil {
		return fmt.Errorf("create postings file: %w", err)
	}
	openFiles = append(openFiles, postings)
	documentLengths, err := os.Create(filepath.Join(destination, "doclens.bin"))
	if err != nil {
		return fmt.Errorf("create document lengths file: %w", err)
	}
	openFiles = append(openFiles, documentLengths)
	documentOffsets, err := os.Create(filepath.Join(destination, "docids.off"))
	if err != nil {
		return fmt.Errorf("create document offsets file: %w", err)
	}
	openFiles = append(openFiles, documentOffsets)
	documentData, err := os.Create(filepath.Join(destination, "docids.dat"))
	if err != nil {
		return fmt.Errorf("create document data file: %w", err)
	}
	openFiles = append(openFiles, documentData)

	termMetadata, err := writePersistentTermFiles(run, terms, postings)
	if err != nil {
		return fmt.Errorf("write term files: %w", err)
	}
	documentMetadata, err := writePersistentDocumentFiles(
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

	metadata, err := os.Create(filepath.Join(destination, "index.meta"))
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
	sidecar io.Reader,
	lengthOutput io.Writer,
	offsetOutput io.Writer,
	dataOutput io.Writer,
) (indexfile.DocumentFilesMetadata, error) {
	documents, err := newDocumentReader(sidecar)
	if err != nil {
		return indexfile.DocumentFilesMetadata{}, err
	}
	return indexfile.WriteDocumentFiles(lengthOutput, offsetOutput, dataOutput, documents.next)
}

func writePersistentTermFiles(
	run io.Reader,
	termOutput io.Writer,
	postingOutput io.Writer,
) (indexfile.TermFilesMetadata, error) {
	terms, err := newRunReader(run)
	if err != nil {
		return indexfile.TermFilesMetadata{}, err
	}
	return indexfile.WriteTermFiles(termOutput, postingOutput, terms.nextTerm, terms.nextPosting)
}
