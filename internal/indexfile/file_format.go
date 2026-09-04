package indexfile

import (
	"fmt"
	"io"
)

type fileRole string

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
