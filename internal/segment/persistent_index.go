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
