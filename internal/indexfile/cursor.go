package indexfile

import (
	"errors"
	"io"

	"github.com/Exclearf/diskseek/internal/index"
)

type Cursor struct {
	input             io.ReaderAt
	term              termEntry
	codec             PostingsCodec
	documentLengths   []uint32
	nextBlockOffset   uint64
	postingsRemaining uint64
	encoded           [maxVBytePostingPayloadBytes]byte
	postings          [postingsPerBlock]index.Posting
	blockPostingCount int
	blockPosition     int
	stats             CursorStats
}

type CursorStats struct {
	NextCalls        uint64
	AdvanceCalls     uint64
	BlockHeadersRead uint64
	BlocksSkipped    uint64
	BlocksDecoded    uint64
	PostingsDecoded  uint64
	BytesRequested   uint64
}

func (i *Index) Postings(term string) (*Cursor, bool, error) {
	entry, found := i.terms[term]
	if !found {
		return nil, false, nil
	}

	cursor := &Cursor{
		input:             i.postings,
		term:              entry,
		codec:             i.postingsCodec,
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

func (c *Cursor) DocumentFrequency() uint64 {
	return c.term.documentFrequency
}

func (c *Cursor) MaxTermFrequency() uint32 {
	return c.term.maxTermFrequency
}

func (c *Cursor) Stats() CursorStats {
	return c.stats
}

func (c *Cursor) Next() (bool, error) {
	c.stats.NextCalls++
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

func (c *Cursor) Advance(target index.DocumentID) (bool, error) {
	c.stats.AdvanceCalls++
	current, valid := c.Current()
	if !valid {
		return false, nil
	}
	if target <= current.DocumentID {
		return true, nil
	}

	lastDocumentID := c.postings[c.blockPostingCount-1].DocumentID
	if target <= lastDocumentID {
		for c.postings[c.blockPosition].DocumentID < target {
			c.blockPosition++
		}
		return true, nil
	}

	c.blockPosition = c.blockPostingCount
	for c.postingsRemaining != 0 {
		header, postingCount, err := c.readBlockHeader()
		if err != nil {
			c.postingsRemaining = 0
			return false, err
		}

		if header.lastDocumentID < target {
			if err := c.consumeBlock(header, postingCount); err != nil {
				return false, err
			}
			c.stats.BlocksSkipped++
			lastDocumentID = header.lastDocumentID
			continue
		}

		if err := c.readBlockPayload(header, postingCount); err != nil {
			c.postingsRemaining = 0
			return false, err
		}
		if c.postings[0].DocumentID <= lastDocumentID {
			c.postingsRemaining = 0
			return false, errors.New("posting document IDs are not strictly increasing across blocks")
		}
		if err := c.consumeBlock(header, postingCount); err != nil {
			return false, err
		}

		c.blockPostingCount = postingCount
		c.blockPosition = 0
		for c.postings[c.blockPosition].DocumentID < target {
			c.blockPosition++
		}
		return true, nil
	}
	return false, nil
}

func (c *Cursor) loadBlock() error {
	header, postingCount, err := c.readBlockHeader()
	if err != nil {
		return err
	}
	if err := c.readBlockPayload(header, postingCount); err != nil {
		return err
	}

	if err := c.consumeBlock(header, postingCount); err != nil {
		return err
	}

	c.blockPostingCount = postingCount
	c.blockPosition = 0
	return nil
}

func (c *Cursor) readBlockHeader() (postingBlockHeader, int, error) {
	postingCount := int(min(c.postingsRemaining, uint64(postingsPerBlock)))
	c.stats.BlockHeadersRead++
	c.stats.BytesRequested += postingBlockHeaderBytes
	header, err := readPostingBlockHeaderAt(
		c.input,
		c.term,
		c.nextBlockOffset,
		postingCount,
		uint64(len(c.documentLengths)),
		c.codec,
		c.encoded[:],
	)
	return header, postingCount, err
}

func (c *Cursor) readBlockPayload(header postingBlockHeader, postingCount int) error {
	c.stats.BytesRequested += uint64(header.payloadBytes)
	if err := readPostingBlockPayloadAt(
		c.input,
		c.nextBlockOffset,
		header,
		c.codec,
		c.documentLengths,
		c.encoded[:header.payloadBytes],
		c.postings[:postingCount],
	); err != nil {
		return err
	}
	c.stats.BlocksDecoded++
	c.stats.PostingsDecoded += uint64(postingCount)
	return nil
}

func (c *Cursor) consumeBlock(header postingBlockHeader, postingCount int) error {
	c.nextBlockOffset += uint64(postingBlockHeaderBytes) + uint64(header.payloadBytes)
	c.postingsRemaining -= uint64(postingCount)
	if c.postingsRemaining == 0 && c.nextBlockOffset-c.term.postingsOffset != c.term.postingsBytes {
		return errors.New("posting list does not end at its term boundary")
	}
	return nil
}
