package indexfile

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	metadataBodyBytes  = 68
	analyzerContractID = 1

	fileMetadataRecordBytes = 12
)

const (
	metadataFileBytes                      = fileHeaderBytes + metadataBodyBytes + fileFooterBytes
	minimumFileBytes                       = fileHeaderBytes + fileFooterBytes
	minimumDocumentOffsetsFileBytes        = minimumFileBytes + documentOffsetBytes
	maximumFileBytes                uint64 = 1<<63 - 1
)

type indexMetadata struct {
	Terms     TermFilesMetadata
	Documents DocumentFilesMetadata
}

func WriteMetadataFile(
	output io.Writer,
	terms TermFilesMetadata,
	documents DocumentFilesMetadata,
) error {
	writer, err := newFileWriter(output, metadataRole)
	if err != nil {
		return err
	}

	var body [metadataBodyBytes]byte
	binary.LittleEndian.PutUint32(body[0:4], analyzerContractID)
	binary.LittleEndian.PutUint32(body[4:8], rawPostingsCodecID)

	files := [...]FileMetadata{
		terms.Terms,
		terms.Postings,
		documents.Lengths,
		documents.Offsets,
		documents.Data,
	}
	for position, file := range files {
		offset := 8 + position*fileMetadataRecordBytes
		binary.LittleEndian.PutUint64(body[offset:offset+8], file.Length)
		binary.LittleEndian.PutUint32(body[offset+8:offset+12], file.Checksum)
	}

	if _, err := writer.Write(body[:]); err != nil {
		return fmt.Errorf("write metadata body: %w", err)
	}
	if _, err := writer.finish(); err != nil {
		return fmt.Errorf("finish metadata: %w", err)
	}
	return nil
}

func readMetadataFile(input io.Reader, size int64) (indexMetadata, error) {
	if size != metadataFileBytes {
		return indexMetadata{}, fmt.Errorf("metadata file size is %d, want %d", size, metadataFileBytes)
	}

	reader, err := newFileReader(input, size, metadataRole)
	if err != nil {
		return indexMetadata{}, err
	}
	var body [metadataBodyBytes]byte
	if _, err := io.ReadFull(reader, body[:]); err != nil {
		return indexMetadata{}, fmt.Errorf("read metadata body: %w", err)
	}
	if err := reader.finish(); err != nil {
		return indexMetadata{}, fmt.Errorf("finish metadata: %w", err)
	}

	if analyzerID := binary.LittleEndian.Uint32(body[0:4]); analyzerID != analyzerContractID {
		return indexMetadata{}, fmt.Errorf("unsupported analyzer ID %d", analyzerID)
	}
	if codecID := binary.LittleEndian.Uint32(body[4:8]); codecID != rawPostingsCodecID {
		return indexMetadata{}, fmt.Errorf("unsupported postings codec ID %d", codecID)
	}

	metadata := indexMetadata{
		Terms: TermFilesMetadata{
			Terms:    decodeFileMetadata(body[8:20]),
			Postings: decodeFileMetadata(body[20:32]),
		},
		Documents: DocumentFilesMetadata{
			Lengths: decodeFileMetadata(body[32:44]),
			Offsets: decodeFileMetadata(body[44:56]),
			Data:    decodeFileMetadata(body[56:68]),
		},
	}
	files := [...]struct {
		name     string
		metadata FileMetadata
		minimum  uint64
	}{
		{TermsFileName, metadata.Terms.Terms, minimumFileBytes},
		{PostingsFileName, metadata.Terms.Postings, minimumFileBytes},
		{DocumentLengthsFileName, metadata.Documents.Lengths, minimumFileBytes},
		{DocumentOffsetsFileName, metadata.Documents.Offsets, minimumDocumentOffsetsFileBytes},
		{DocumentDataFileName, metadata.Documents.Data, minimumFileBytes},
	}
	for _, file := range files {
		if file.metadata.Length < file.minimum || file.metadata.Length > maximumFileBytes {
			return indexMetadata{}, fmt.Errorf("invalid %s length %d", file.name, file.metadata.Length)
		}
	}
	return metadata, nil
}

func decodeFileMetadata(data []byte) FileMetadata {
	return FileMetadata{
		Length:   binary.LittleEndian.Uint64(data[0:8]),
		Checksum: binary.LittleEndian.Uint32(data[8:12]),
	}
}

const rawPostingsCodecID = 1
