package indexfile

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

type fileRole string

const (
	MetadataFileName        = "index.meta"
	TermsFileName           = "terms.bin"
	PostingsFileName        = "postings.bin"
	DocumentLengthsFileName = "doclens.bin"
	DocumentOffsetsFileName = "docids.off"
	DocumentDataFileName    = "docids.dat"
)

const (
	fileRoleBytes                = 7
	fileHeaderBytes              = fileRoleBytes + 1
	fileFormatVersion            = 1
	metadataRole        fileRole = "DSKMETA"
	termsRole           fileRole = "DSKTERM"
	postingsRole        fileRole = "DSKPOST"
	documentLengthsRole fileRole = "DSKDLEN"
	documentOffsetsRole fileRole = "DSKDOFF"
	documentDataRole    fileRole = "DSKDDAT"
)

const fileFooterBytes = 4

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

func writeHeader(writer io.Writer, role fileRole) error {
	var header [fileHeaderBytes]byte
	copy(header[:fileRoleBytes], role)
	header[fileRoleBytes] = fileFormatVersion
	if _, err := writer.Write(header[:]); err != nil {
		return fmt.Errorf("write %s header: %w", role, err)
	}
	return nil
}

func readHeader(reader io.Reader, expectedRole fileRole) error {
	var header [fileHeaderBytes]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return fmt.Errorf("read %s header: %w", expectedRole, err)
	}

	role := fileRole(header[:fileRoleBytes])
	if role != expectedRole {
		return fmt.Errorf("read %s header: unexpected role %q", expectedRole, role)
	}
	if header[fileRoleBytes] != fileFormatVersion {
		return fmt.Errorf(
			"read %s header: unsupported format version %d",
			expectedRole,
			header[fileRoleBytes],
		)
	}
	return nil
}

func writeFooter(writer io.Writer, checksum uint32) error {
	var footer [fileFooterBytes]byte
	binary.LittleEndian.PutUint32(footer[:], checksum)
	if _, err := writer.Write(footer[:]); err != nil {
		return fmt.Errorf("write checksum footer: %w", err)
	}
	return nil
}

func readFooter(reader io.Reader) (uint32, error) {
	var footer [fileFooterBytes]byte
	if _, err := io.ReadFull(reader, footer[:]); err != nil {
		return 0, fmt.Errorf("read checksum footer: %w", err)
	}
	return binary.LittleEndian.Uint32(footer[:]), nil
}
