package segment

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/corpus"
	"github.com/Exclearf/diskseek/internal/index"
)

const documentMagic = "DSKDOC01"

type documentWriter struct {
	output io.WriteCloser
	writer *bufio.Writer
	err    error
	closed bool
}

func newDocumentWriter(output io.WriteCloser) (*documentWriter, error) {
	w := &documentWriter{
		output: output,
		writer: bufio.NewWriterSize(output, runBufferBytes),
	}
	if _, err := io.WriteString(w.writer, documentMagic); err != nil {
		return nil, errors.Join(fmt.Errorf("write document magic: %w", err), output.Close())
	}
	return w, nil
}

func (w *documentWriter) write(document index.DocumentMeta) error {
	if w.closed {
		return errors.New("document writer is closed")
	}
	if w.err != nil {
		return w.err
	}
	if len(document.ExternalID) == 0 || len(document.ExternalID) > corpus.MaxExternalIDBytes ||
		!utf8.ValidString(document.ExternalID) {
		return w.fail(errors.New("invalid external document ID"))
	}

	if err := writeUint32(w.writer, uint32(len(document.ExternalID))); err != nil {
		return w.fail(fmt.Errorf("write external document ID length: %w", err))
	}
	if _, err := io.WriteString(w.writer, document.ExternalID); err != nil {
		return w.fail(fmt.Errorf("write external document ID: %w", err))
	}
	if err := writeUint32(w.writer, document.Length); err != nil {
		return w.fail(fmt.Errorf("write analyzed document length: %w", err))
	}
	return nil
}

func (w *documentWriter) close() error {
	if w.closed {
		return errors.New("document writer is already closed")
	}
	w.closed = true

	var endErr error
	if w.err == nil {
		endErr = writeUint32(w.writer, 0)
	}
	flushErr := w.writer.Flush()
	closeErr := w.output.Close()
	return errors.Join(w.err, endErr, flushErr, closeErr)
}

func (w *documentWriter) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	return err
}

type documentReader struct {
	reader *bufio.Reader
}

func newDocumentReader(input io.Reader) (*documentReader, error) {
	reader := bufio.NewReaderSize(input, runBufferBytes)
	var magic [len(documentMagic)]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return nil, fmt.Errorf("read document magic: %w", err)
	}
	if string(magic[:]) != documentMagic {
		return nil, errors.New("invalid document magic")
	}
	return &documentReader{reader: reader}, nil
}

func (r *documentReader) next() (index.DocumentMeta, error) {
	externalIDLength, err := readUint32(r.reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return index.DocumentMeta{}, fmt.Errorf("read external document ID length: %w", err)
	}
	if externalIDLength == 0 {
		var trailing [1]byte
		if _, err := io.ReadFull(r.reader, trailing[:]); errors.Is(err, io.EOF) {
			return index.DocumentMeta{}, io.EOF
		} else if err != nil {
			return index.DocumentMeta{}, fmt.Errorf("check document sidecar end: %w", err)
		}
		return index.DocumentMeta{}, errors.New("document sidecar has trailing bytes")
	}
	if externalIDLength > corpus.MaxExternalIDBytes {
		return index.DocumentMeta{}, errors.New("invalid external document ID length")
	}

	externalID := make([]byte, int(externalIDLength))
	if _, err := io.ReadFull(r.reader, externalID); err != nil {
		return index.DocumentMeta{}, fmt.Errorf("read external document ID: %w", err)
	}
	if !utf8.Valid(externalID) {
		return index.DocumentMeta{}, errors.New("external document ID is not valid UTF-8")
	}
	documentLength, err := readUint32(r.reader)
	if err != nil {
		return index.DocumentMeta{}, fmt.Errorf("read analyzed document length: %w", err)
	}
	return index.DocumentMeta{ExternalID: string(externalID), Length: documentLength}, nil
}

func writeUint32(writer io.Writer, value uint32) error {
	var encoded [4]byte
	binary.LittleEndian.PutUint32(encoded[:], value)
	_, err := writer.Write(encoded[:])
	return err
}

func readUint32(reader io.Reader) (uint32, error) {
	var encoded [4]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(encoded[:]), nil
}
