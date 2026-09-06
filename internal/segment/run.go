package segment

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	runMagic            = "DSKRUN01"
	runHeaderBytes      = 20
	runBufferBytes      = 32 << 10
	encodedPostingBytes = 8
	maxRunTermBytes     = 1 << 20
	documentIDLimit     = uint64(1) << 32
)

type runHeader struct {
	firstDocumentID index.DocumentID
	documentCount   uint64
}

type runWriter struct {
	output            io.WriteCloser
	writer            *bufio.Writer
	header            runHeader
	postingsRemaining uint64
	lastTerm          string
	lastDocumentID    index.DocumentID
	hasLastDocumentID bool
	encodedPosting    [encodedPostingBytes]byte
	err               error
	closed            bool
}

func newRunWriter(output io.WriteCloser, header runHeader) (*runWriter, error) {
	w := &runWriter{
		output: output,
		writer: bufio.NewWriterSize(output, runBufferBytes),
		header: header,
	}
	if err := writeRunHeader(w.writer, header); err != nil {
		return nil, errors.Join(err, output.Close())
	}
	return w, nil
}

func (w *runWriter) writeTerm(term string, postingCount uint64) error {
	if err := w.ready(); err != nil {
		return err
	}
	if w.postingsRemaining != 0 {
		return w.fail(errors.New("previous run term has unwritten postings"))
	}
	if w.lastTerm != "" && term <= w.lastTerm {
		return w.fail(errors.New("run terms are not strictly increasing"))
	}
	if err := writeRunTermHeader(w.writer, w.header, term, postingCount); err != nil {
		return w.fail(err)
	}

	w.postingsRemaining = postingCount
	w.lastTerm = term
	w.hasLastDocumentID = false
	return nil
}

func (w *runWriter) writePosting(posting index.Posting) error {
	if err := w.ready(); err != nil {
		return err
	}
	if w.postingsRemaining == 0 {
		return w.fail(errors.New("run posting has no active term"))
	}
	if w.hasLastDocumentID && posting.DocumentID <= w.lastDocumentID {
		return w.fail(errors.New("run posting document IDs are not strictly increasing"))
	}
	if err := writeRunPosting(w.writer, w.encodedPosting[:], w.header, posting); err != nil {
		return w.fail(err)
	}

	w.postingsRemaining--
	w.lastDocumentID = posting.DocumentID
	w.hasLastDocumentID = true
	return nil
}

func (w *runWriter) close() error {
	if w.closed {
		return errors.New("run writer is already closed")
	}
	w.closed = true

	var stateErr error
	if w.postingsRemaining != 0 {
		stateErr = errors.New("run is incomplete")
	}
	var endErr error
	if w.err == nil && stateErr == nil {
		var end [4]byte
		_, endErr = w.writer.Write(end[:])
	}
	flushErr := w.writer.Flush()
	closeErr := w.output.Close()
	return errors.Join(w.err, stateErr, endErr, flushErr, closeErr)
}

func (w *runWriter) ready() error {
	if w.closed {
		return errors.New("run writer is closed")
	}
	return w.err
}

func (w *runWriter) fail(err error) error {
	if w.err == nil {
		w.err = err
	}
	return err
}

type runReader struct {
	reader            *bufio.Reader
	header            runHeader
	postingsRemaining uint64
	lastTerm          string
	lastDocumentID    index.DocumentID
	hasLastDocumentID bool
	encodedPosting    [encodedPostingBytes]byte
}

func newRunReader(input io.Reader) (*runReader, error) {
	reader := bufio.NewReaderSize(input, runBufferBytes)
	header, err := readRunHeader(reader)
	if err != nil {
		return nil, err
	}
	return &runReader{reader: reader, header: header}, nil
}

func (r *runReader) nextTerm() (string, uint64, error) {
	if r.postingsRemaining != 0 {
		return "", 0, errors.New("current run term has unread postings")
	}

	term, postingCount, err := readRunTermHeader(r.reader, r.header)
	if err != nil {
		return "", 0, err
	}
	if r.lastTerm != "" && term <= r.lastTerm {
		return "", 0, errors.New("run terms are not strictly increasing")
	}
	r.postingsRemaining = postingCount
	r.lastTerm = term
	r.hasLastDocumentID = false
	return term, postingCount, nil
}

func (r *runReader) nextPosting() (index.Posting, error) {
	if r.postingsRemaining == 0 {
		return index.Posting{}, errors.New("run posting has no active term")
	}

	posting, err := readRunPosting(r.reader, r.encodedPosting[:], r.header)
	if err != nil {
		return index.Posting{}, err
	}
	if r.hasLastDocumentID && posting.DocumentID <= r.lastDocumentID {
		return index.Posting{}, errors.New("run posting document IDs are not strictly increasing")
	}
	r.postingsRemaining--
	r.lastDocumentID = posting.DocumentID
	r.hasLastDocumentID = true
	return posting, nil
}

func writeRunHeader(writer io.Writer, header runHeader) error {
	if err := validateRunHeader(header); err != nil {
		return err
	}

	var encoded [runHeaderBytes]byte
	copy(encoded[:8], runMagic)
	binary.LittleEndian.PutUint32(encoded[8:12], uint32(header.firstDocumentID))
	binary.LittleEndian.PutUint64(encoded[12:20], header.documentCount)
	if _, err := writer.Write(encoded[:]); err != nil {
		return fmt.Errorf("write run header: %w", err)
	}
	return nil
}

func readRunHeader(reader io.Reader) (runHeader, error) {
	var encoded [runHeaderBytes]byte
	if _, err := io.ReadFull(reader, encoded[:]); err != nil {
		return runHeader{}, fmt.Errorf("read run header: %w", err)
	}
	if string(encoded[:8]) != runMagic {
		return runHeader{}, errors.New("invalid run magic")
	}

	header := runHeader{
		firstDocumentID: index.DocumentID(binary.LittleEndian.Uint32(encoded[8:12])),
		documentCount:   binary.LittleEndian.Uint64(encoded[12:20]),
	}
	if err := validateRunHeader(header); err != nil {
		return runHeader{}, err
	}
	return header, nil
}

func writeRunTermHeader(writer io.Writer, run runHeader, term string, postingCount uint64) error {
	if len(term) == 0 || len(term) > maxRunTermBytes || !utf8.ValidString(term) {
		return errors.New("invalid run term")
	}
	if postingCount == 0 || postingCount > run.documentCount {
		return errors.New("invalid run posting count")
	}

	var termLength [4]byte
	binary.LittleEndian.PutUint32(termLength[:], uint32(len(term)))
	if _, err := writer.Write(termLength[:]); err != nil {
		return fmt.Errorf("write run term length: %w", err)
	}
	if _, err := io.WriteString(writer, term); err != nil {
		return fmt.Errorf("write run term: %w", err)
	}

	var encodedCount [8]byte
	binary.LittleEndian.PutUint64(encodedCount[:], postingCount)
	if _, err := writer.Write(encodedCount[:]); err != nil {
		return fmt.Errorf("write run posting count: %w", err)
	}
	return nil
}

func readRunTermHeader(reader io.Reader, run runHeader) (string, uint64, error) {
	var encodedLength [4]byte
	if _, err := io.ReadFull(reader, encodedLength[:]); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", 0, fmt.Errorf("read run term length: %w", err)
	}

	termLength := binary.LittleEndian.Uint32(encodedLength[:])
	if termLength == 0 {
		var trailing [1]byte
		if _, err := io.ReadFull(reader, trailing[:]); errors.Is(err, io.EOF) {
			return "", 0, io.EOF
		} else if err != nil {
			return "", 0, fmt.Errorf("check run end: %w", err)
		}
		return "", 0, errors.New("run has trailing bytes")
	}
	if termLength > maxRunTermBytes {
		return "", 0, errors.New("invalid run term length")
	}

	termBytes := make([]byte, int(termLength))
	if _, err := io.ReadFull(reader, termBytes); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", 0, fmt.Errorf("read run term: %w", err)
	}
	if !utf8.Valid(termBytes) {
		return "", 0, errors.New("run term is not valid UTF-8")
	}

	var encodedCount [8]byte
	if _, err := io.ReadFull(reader, encodedCount[:]); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", 0, fmt.Errorf("read run posting count: %w", err)
	}
	postingCount := binary.LittleEndian.Uint64(encodedCount[:])
	if postingCount == 0 || postingCount > run.documentCount {
		return "", 0, errors.New("invalid run posting count")
	}
	return string(termBytes), postingCount, nil
}

func writeRunPosting(writer io.Writer, encoded []byte, run runHeader, posting index.Posting) error {
	if posting.Frequency == 0 {
		return errors.New("run posting has zero frequency")
	}
	if !documentInRun(run, posting.DocumentID) {
		return errors.New("run posting document ID is outside the run")
	}

	binary.LittleEndian.PutUint32(encoded[0:4], uint32(posting.DocumentID))
	binary.LittleEndian.PutUint32(encoded[4:8], posting.Frequency)
	if _, err := writer.Write(encoded); err != nil {
		return fmt.Errorf("write run posting: %w", err)
	}
	return nil
}

func readRunPosting(reader io.Reader, encoded []byte, run runHeader) (index.Posting, error) {
	if _, err := io.ReadFull(reader, encoded); err != nil {
		return index.Posting{}, fmt.Errorf("read run posting: %w", err)
	}

	posting := index.Posting{
		DocumentID: index.DocumentID(binary.LittleEndian.Uint32(encoded[0:4])),
		Frequency:  binary.LittleEndian.Uint32(encoded[4:8]),
	}
	if posting.Frequency == 0 {
		return index.Posting{}, errors.New("run posting has zero frequency")
	}
	if !documentInRun(run, posting.DocumentID) {
		return index.Posting{}, errors.New("run posting document ID is outside the run")
	}
	return posting, nil
}

func validateRunHeader(header runHeader) error {
	if header.documentCount == 0 {
		if header.firstDocumentID != 0 {
			return errors.New("invalid empty run header")
		}
		return nil
	}

	start := uint64(header.firstDocumentID)
	if header.documentCount > documentIDLimit-start {
		return errors.New("run document interval overflows")
	}
	return nil
}

func documentInRun(header runHeader, documentID index.DocumentID) bool {
	value := uint64(documentID)
	start := uint64(header.firstDocumentID)
	return value >= start && value-start < header.documentCount
}
