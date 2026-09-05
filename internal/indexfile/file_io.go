package indexfile

import (
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

type fileReader struct {
	input    io.Reader
	body     io.LimitedReader
	checksum hash.Hash32
}

func newFileReader(input io.Reader, size int64, role fileRole) (*fileReader, error) {
	if size < fileHeaderBytes+fileFooterBytes {
		return nil, fmt.Errorf("read %s file: size %d is too small", role, size)
	}

	checksum := crc32.New(crc32cTable)
	checksummed := io.TeeReader(input, checksum)
	if err := readHeader(checksummed, role); err != nil {
		return nil, err
	}

	return &fileReader{
		input:    input,
		body:     io.LimitedReader{R: checksummed, N: size - fileHeaderBytes - fileFooterBytes},
		checksum: checksum,
	}, nil
}

func (r *fileReader) Read(data []byte) (int, error) {
	return r.body.Read(data)
}

func (r *fileReader) finish() error {
	if r.body.N != 0 {
		return fmt.Errorf("file body has %d unread bytes", r.body.N)
	}

	stored, err := readFooter(r.input)
	if err != nil {
		return err
	}
	checksum := r.checksum.Sum32()
	if stored != checksum {
		return fmt.Errorf("file checksum is %08x, want %08x", stored, checksum)
	}
	return nil
}

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
