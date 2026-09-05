package indexfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type Index struct {
	terms                    map[string]termEntry
	postingsCodec            PostingsCodec
	documentLengths          []uint32
	documentsWithTerms       uint64
	totalLength              uint64
	averageDocumentLength    float64
	postings                 indexFile
	documentOffsets          indexFile
	documentOffsetsBodyBytes uint64
	documentData             indexFile
	documentDataBodyBytes    uint64
}

type indexFile interface {
	io.Reader
	io.ReaderAt
	io.Closer
	Stat() (fs.FileInfo, error)
}

type openedIndexFile struct {
	file   indexFile
	size   int64
	closed bool
}

func Open(directory string) (*Index, error) {
	return openIndex(directory, func(path string) (indexFile, error) {
		return os.Open(path)
	})
}

func openIndex(
	directory string,
	openFile func(string) (indexFile, error),
) (result *Index, err error) {
	names := [...]string{
		MetadataFileName,
		TermsFileName,
		PostingsFileName,
		DocumentLengthsFileName,
		DocumentOffsetsFileName,
		DocumentDataFileName,
	}
	files := make(map[string]*openedIndexFile, len(names))
	defer func() {
		if err == nil {
			return
		}
		for _, name := range names {
			if openedFile := files[name]; openedFile != nil && !openedFile.closed {
				err = errors.Join(err, openedFile.file.Close())
				openedFile.closed = true
			}
		}
	}()

	for _, name := range names {
		file, openErr := openFile(filepath.Join(directory, name))
		if openErr != nil {
			return nil, fmt.Errorf("open %s: %w", name, openErr)
		}
		openedFile := &openedIndexFile{file: file}
		files[name] = openedFile

		info, statErr := file.Stat()
		if statErr != nil {
			return nil, fmt.Errorf("stat %s: %w", name, statErr)
		}
		openedFile.size = info.Size()
	}

	metadataFile := files[MetadataFileName]
	metadata, err := readMetadataFile(metadataFile.file, metadataFile.size)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", MetadataFileName, err)
	}
	manifests := [...]struct {
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
	for _, manifest := range manifests {
		file := files[manifest.name]
		if err := verifyManifest(file.file, file.size, manifest.role, manifest.metadata); err != nil {
			return nil, fmt.Errorf("verify %s: %w", manifest.name, err)
		}
	}

	lengthFile := files[DocumentLengthsFileName]
	lengths, err := readDocumentLengths(lengthFile.file, lengthFile.size)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", DocumentLengthsFileName, err)
	}
	postingFile := files[PostingsFileName]
	postingBodyBytes := uint64(postingFile.size - minimumFileBytes)
	termFile := files[TermsFileName]
	terms, err := readTermFile(
		termFile.file,
		termFile.size,
		postingBodyBytes,
		lengths.documentsWithTerms,
	)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", TermsFileName, err)
	}

	offsetFile := files[DocumentOffsetsFileName]
	dataFile := files[DocumentDataFileName]

	for _, name := range [...]string{MetadataFileName, TermsFileName, DocumentLengthsFileName} {
		file := files[name]
		file.closed = true
		if err := file.file.Close(); err != nil {
			return nil, fmt.Errorf("close %s: %w", name, err)
		}
	}

	var averageDocumentLength float64
	if lengths.documentsWithTerms != 0 {
		averageDocumentLength = float64(lengths.totalLength) / float64(lengths.documentsWithTerms)
	}
	return &Index{
		terms:                    terms,
		postingsCodec:            metadata.Terms.Codec,
		documentLengths:          lengths.values,
		documentsWithTerms:       lengths.documentsWithTerms,
		totalLength:              lengths.totalLength,
		averageDocumentLength:    averageDocumentLength,
		postings:                 postingFile.file,
		documentOffsets:          offsetFile.file,
		documentOffsetsBodyBytes: uint64(offsetFile.size - minimumFileBytes),
		documentData:             dataFile.file,
		documentDataBodyBytes:    uint64(dataFile.size - minimumFileBytes),
	}, nil
}

func (i *Index) Close() error {
	return errors.Join(
		i.documentData.Close(),
		i.documentOffsets.Close(),
		i.postings.Close(),
	)
}
