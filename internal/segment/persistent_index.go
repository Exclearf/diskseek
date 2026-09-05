package segment

import (
	"io"

	"github.com/Exclearf/diskseek/internal/indexfile"
)

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
