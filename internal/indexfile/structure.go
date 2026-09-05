package indexfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func verifyIndexStructure(directory string) (indexMetadata, error) {
	metadata, err := readIndexMetadataFile(filepath.Join(directory, MetadataFileName))
	if err != nil {
		return indexMetadata{}, fmt.Errorf("verify index.meta: %w", err)
	}

	files := [...]struct {
		name     string
		role     fileRole
		metadata FileMetadata
	}{
		{TermsFileName, termsRole, metadata.Terms.Terms},
		{PostingsFileName, postingsRole, metadata.Terms.Postings},
		{DocumentLengthsFileName, documentLengthsRole, metadata.Documents.Lengths},
		{DocumentOffsetsFileName, documentOffsetsRole, metadata.Documents.Offsets},
		{DocumentDataFileName, documentDataRole, metadata.Documents.Data},
	}
	for _, file := range files {
		if err := verifyManifestFile(
			filepath.Join(directory, file.name),
			file.role,
			file.metadata,
		); err != nil {
			return indexMetadata{}, fmt.Errorf("verify %s: %w", file.name, err)
		}
	}
	return metadata, nil
}

func readIndexMetadataFile(path string) (metadata indexMetadata, err error) {
	input, err := os.Open(path)
	if err != nil {
		return indexMetadata{}, err
	}
	defer func() {
		err = errors.Join(err, input.Close())
	}()

	info, err := input.Stat()
	if err != nil {
		return indexMetadata{}, err
	}
	return readMetadataFile(input, info.Size())
}

func verifyManifestFile(path string, role fileRole, metadata FileMetadata) (err error) {
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, input.Close())
	}()

	info, err := input.Stat()
	if err != nil {
		return err
	}
	return verifyManifest(input, info.Size(), role, metadata)
}

func verifyManifest(input io.ReaderAt, size int64, role fileRole, metadata FileMetadata) error {
	if size != int64(metadata.Length) {
		return fmt.Errorf("file size is %d, want %d", size, metadata.Length)
	}
	header := io.NewSectionReader(input, 0, fileHeaderBytes)
	if err := readHeader(header, role); err != nil {
		return err
	}
	footer := io.NewSectionReader(input, size-fileFooterBytes, fileFooterBytes)
	stored, err := readFooter(footer)
	if err != nil {
		return err
	}
	if stored != metadata.Checksum {
		return fmt.Errorf("stored checksum is %08x, want %08x", stored, metadata.Checksum)
	}
	return nil
}
