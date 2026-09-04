package indexfile

import (
	"hash"
	"hash/crc32"
	"io"
)

type fileWriter struct {
	output      io.Writer
	checksummed io.Writer
	checksum    hash.Hash32
}

func newFileWriter(output io.Writer, role fileRole) (*fileWriter, error) {
	checksum := crc32.New(crc32cTable)
	w := &fileWriter{
		output:      output,
		checksummed: io.MultiWriter(output, checksum),
		checksum:    checksum,
	}
	if err := writeHeader(w.checksummed, role); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *fileWriter) Write(data []byte) (int, error) {
	return w.checksummed.Write(data)
}

func (w *fileWriter) finish() (uint32, error) {
	checksum := w.checksum.Sum32()
	if err := writeFooter(w.output, checksum); err != nil {
		return 0, err
	}
	return checksum, nil
}
