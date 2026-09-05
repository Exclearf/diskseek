package indexfile

import (
	"errors"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

type Cursor struct {
	input             io.ReaderAt
	term              termEntry
	documentLengths   []uint32
	nextBlockOffset   uint64
	postingsRemaining uint64
	encoded           [rawPostingsPerBlock * rawPostingBytes]byte
	postings          [rawPostingsPerBlock]index.Posting
	blockPostingCount int
	blockPosition     int
}

func (i *Index) Postings(term string) (*Cursor, bool, error) {
	entry, found := i.terms[term]
	if !found {
		return nil, false, nil
	}

	cursor := &Cursor{
		input:             i.postings,
		term:              entry,
		documentLengths:   i.documentLengths,
		nextBlockOffset:   entry.postingsOffset,
		postingsRemaining: entry.documentFrequency,
	}
	if err := cursor.loadBlock(); err != nil {
		return nil, true, err
	}
	return cursor, true, nil
}

func (c *Cursor) Current() (index.Posting, bool) {
	if c.blockPosition >= c.blockPostingCount {
		return index.Posting{}, false
	}
	return c.postings[c.blockPosition], true
}

func (c *Cursor) Next() (bool, error) {
	if c.blockPosition >= c.blockPostingCount {
		return false, nil
	}

	c.blockPosition++
	if c.blockPosition < c.blockPostingCount {
		return true, nil
	}
	if c.postingsRemaining == 0 {
		return false, nil
	}
	previousDocumentID := c.postings[c.blockPosition-1].DocumentID
	if err := c.loadBlock(); err != nil {
		c.postingsRemaining = 0
		return false, err
	}
	if c.postings[0].DocumentID <= previousDocumentID {
		c.blockPosition = c.blockPostingCount
		c.postingsRemaining = 0
		return false, errors.New("posting document IDs are not strictly increasing across blocks")
	}
	return true, nil
}

func (c *Cursor) loadBlock() error {
	postingCount := int(min(c.postingsRemaining, uint64(rawPostingsPerBlock)))
	header, err := readRawPostingBlockHeaderAt(
		c.input,
		c.term,
		c.nextBlockOffset,
		postingCount,
		uint64(len(c.documentLengths)),
		c.encoded[:],
	)
	if err != nil {
		return err
	}
	if err := readRawPostingBlockPayloadAt(
		c.input,
		c.nextBlockOffset,
		header,
		c.documentLengths,
		c.encoded[:header.payloadBytes],
		c.postings[:postingCount],
	); err != nil {
		return err
	}

	c.nextBlockOffset += uint64(postingBlockHeaderBytes) + uint64(header.payloadBytes)
	c.postingsRemaining -= uint64(postingCount)
	if c.postingsRemaining == 0 && c.nextBlockOffset-c.term.postingsOffset != c.term.postingsBytes {
		return errors.New("posting list does not end at its term boundary")
	}

	c.blockPostingCount = postingCount
	c.blockPosition = 0
	return nil
}
