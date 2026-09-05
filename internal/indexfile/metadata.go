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

const rawPostingsCodecID = 1
