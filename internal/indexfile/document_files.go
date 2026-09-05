package indexfile

import (
	"errors"
	"fmt"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

func writeDocumentBodies(
	lengths io.Writer,
	offsets io.Writer,
	data io.Writer,
	nextDocument func() (index.DocumentMeta, error),
) error {
	var offset uint64
	if err := writeDocumentOffset(offsets, offset); err != nil {
		return err
	}

	for {
		document, err := nextDocument()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read document metadata: %w", err)
		}

		if err := writeDocumentLength(lengths, document.Length); err != nil {
			return err
		}
		if _, err := io.WriteString(data, document.ExternalID); err != nil {
			return fmt.Errorf("write external document ID: %w", err)
		}
		offset += uint64(len(document.ExternalID))
		if err := writeDocumentOffset(offsets, offset); err != nil {
			return err
		}
	}
}

type documentFilesMetadata struct {
	lengths fileMetadata
	offsets fileMetadata
	data    fileMetadata
}

func writeDocumentFiles(
	lengthOutput io.Writer,
	offsetOutput io.Writer,
	dataOutput io.Writer,
	nextDocument func() (index.DocumentMeta, error),
) (documentFilesMetadata, error) {
	lengths, err := newFileWriter(lengthOutput, documentLengthsRole)
	if err != nil {
		return documentFilesMetadata{}, err
	}
	offsets, err := newFileWriter(offsetOutput, documentOffsetsRole)
	if err != nil {
		return documentFilesMetadata{}, err
	}
	data, err := newFileWriter(dataOutput, documentDataRole)
	if err != nil {
		return documentFilesMetadata{}, err
	}

	if err := writeDocumentBodies(lengths, offsets, data, nextDocument); err != nil {
		return documentFilesMetadata{}, err
	}
	lengthMetadata, err := lengths.finish()
	if err != nil {
		return documentFilesMetadata{}, fmt.Errorf("finish document lengths: %w", err)
	}
	offsetMetadata, err := offsets.finish()
	if err != nil {
		return documentFilesMetadata{}, fmt.Errorf("finish document offsets: %w", err)
	}
	dataMetadata, err := data.finish()
	if err != nil {
		return documentFilesMetadata{}, fmt.Errorf("finish document data: %w", err)
	}

	return documentFilesMetadata{
		lengths: lengthMetadata,
		offsets: offsetMetadata,
		data:    dataMetadata,
	}, nil
}
