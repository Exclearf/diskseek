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
