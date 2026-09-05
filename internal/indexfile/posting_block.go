package indexfile

import (
	"encoding/binary"
	"errors"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	postingBlockHeaderBytes = 8
	postingsPerBlock        = 128
	maxPostingsPerList      = uint64(1) << 32
)

type postingBlockHeader struct {
	lastDocumentID index.DocumentID
	payloadBytes   uint32
}

func decodePostingBlockHeader(
	encoded []byte,
	totalDocuments uint64,
) (postingBlockHeader, error) {
	lastDocumentID := index.DocumentID(binary.LittleEndian.Uint32(encoded[0:4]))
	if uint64(lastDocumentID) >= totalDocuments {
		return postingBlockHeader{}, errors.New("posting block endpoint is outside the index")
	}
	return postingBlockHeader{
		lastDocumentID: lastDocumentID,
		payloadBytes:   binary.LittleEndian.Uint32(encoded[4:8]),
	}, nil
}

func validatePostingBlock(
	postings []index.Posting,
	header postingBlockHeader,
) error {
	for position, posting := range postings {
		if posting.Frequency == 0 {
			return errors.New("posting has zero frequency")
		}
		if position != 0 && posting.DocumentID <= postings[position-1].DocumentID {
			return errors.New("posting document IDs are not strictly increasing")
		}
	}
	if postings[len(postings)-1].DocumentID != header.lastDocumentID {
		return errors.New("posting block endpoint does not match its payload")
	}
	return nil
}
