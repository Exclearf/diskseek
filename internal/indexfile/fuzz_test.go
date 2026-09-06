package indexfile

import (
	"bytes"
	"io/fs"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	maxFuzzInputBytes = 4 << 10
	maxFuzzDocuments  = 256
	maxFuzzPostings   = 256
	maxFuzzOperations = 256
)

func FuzzReadMetadataFile(f *testing.F) {
	f.Add(readGoldenIndexFile(f, MetadataFileName))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		_, _ = readMetadataFile(bytes.NewReader(data), int64(len(data)))
	})
}

func FuzzReadDocumentLengths(f *testing.F) {
	f.Add(readGoldenIndexFile(f, DocumentLengthsFileName))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		_, _ = readDocumentLengths(bytes.NewReader(data), int64(len(data)))
	})
}

func FuzzReadExternalIDs(f *testing.F) {
	offsets := goldenIndexBody(f, DocumentOffsetsFileName)
	data := goldenIndexBody(f, DocumentDataFileName)
	f.Add(offsets, data, uint64(2))
	f.Fuzz(func(t *testing.T, offsets []byte, data []byte, documentCount uint64) {
		if len(offsets) > maxFuzzInputBytes || len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		documentCount %= maxFuzzDocuments + 1
		_ = readExternalIDs(
			bytes.NewReader(offsets),
			bytes.NewReader(data),
			documentCount,
			uint64(len(offsets)),
			uint64(len(data)),
			func(string) error { return nil },
		)
	})
}

func FuzzReadTerms(f *testing.F) {
	data := goldenIndexBody(f, TermsFileName)
	f.Add(data, uint64(40), uint64(2))
	f.Fuzz(func(t *testing.T, data []byte, postingsBytes uint64, documentsWithTerms uint64) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		postingsBytes %= maxFuzzInputBytes + 1
		documentsWithTerms %= maxFuzzDocuments + 1
		_, _ = readTerms(
			bytes.NewReader(data),
			uint64(len(data)),
			postingsBytes,
			documentsWithTerms,
		)
	})
}

func FuzzReadRawPostingList(f *testing.F) {
	f.Add([]byte(rawPostingBlockFixture), uint64(2), uint64(24), uint64(2))
	for _, postingCount := range []int{1, postingsPerBlock, postingsPerBlock + 1} {
		postings := cursorTestPostings(postingCount)
		data := encodeFuzzPostingList(f, PostingsCodecRaw, postings)
		f.Add(data, uint64(postingCount), uint64(len(data)), uint64(postingCount))
	}
	f.Fuzz(func(
		t *testing.T,
		data []byte,
		postingCount uint64,
		postingBytes uint64,
		documentCount uint64,
	) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		postingCount %= maxFuzzPostings + 1
		postingBytes %= maxFuzzInputBytes + 1
		documentCount %= maxFuzzDocuments + 1
		_ = readRawPostingList(
			bytes.NewReader(data),
			postingCount,
			postingBytes,
			documentCount,
			func(index.Posting) error { return nil },
		)
	})
}

func FuzzReadVBytePostingList(f *testing.F) {
	postings := vBytePostingFixturePostings()
	data := encodeFuzzPostingList(f, PostingsCodecVByte, postings)
	f.Add(data, uint64(len(postings)), uint64(len(data)), uint64(825))
	for _, postingCount := range []int{1, postingsPerBlock, postingsPerBlock + 1} {
		postings := cursorTestPostings(postingCount)
		data := encodeFuzzPostingList(f, PostingsCodecVByte, postings)
		f.Add(data, uint64(postingCount), uint64(len(data)), uint64(postingCount))
	}
	f.Fuzz(func(
		t *testing.T,
		data []byte,
		postingCount uint64,
		postingBytes uint64,
		documentCount uint64,
	) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		postingCount %= maxFuzzPostings + 1
		postingBytes %= maxFuzzInputBytes + 1
		documentCount %= maxFuzzDocuments + 1
		_ = readVBytePostingList(
			bytes.NewReader(data),
			postingCount,
			postingBytes,
			documentCount,
			func(index.Posting) error { return nil },
		)
	})
}

func FuzzDecodeVBytePostingPayload(f *testing.F) {
	f.Add([]byte(vBytePostingPayloadFixture), uint8(4))
	for _, postingCount := range []int{1, postingsPerBlock} {
		postings := cursorTestPostings(postingCount)
		var encoded [maxVBytePostingPayloadBytes]byte
		encodedBytes, err := encodeVBytePostingPayload(encoded[:], postings)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(encoded[:encodedBytes], uint8(postingCount))
	}
	f.Fuzz(func(t *testing.T, data []byte, postingCount uint8) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		count := int(postingCount) % (postingsPerBlock + 1)
		_ = decodeVBytePostingPayload(data, make([]index.Posting, count))
	})
}

func FuzzCursorOperations(f *testing.F) {
	for _, codec := range []PostingsCodec{PostingsCodecRaw, PostingsCodecVByte} {
		for _, postingCount := range []int{1, postingsPerBlock + 1} {
			postings := cursorTestPostings(postingCount)
			f.Add(
				encodeFuzzPostingList(f, codec, postings),
				codec == PostingsCodecVByte,
				uint16(len(postings)),
				uint16(postings[len(postings)-1].DocumentID+1),
				[]byte{0, 0, 1, 127, 1, 128, 1, 255},
			)
		}
	}

	f.Fuzz(func(
		t *testing.T,
		data []byte,
		vbyte bool,
		postingCount uint16,
		documentCount uint16,
		operations []byte,
	) {
		if len(data) > maxFuzzInputBytes || len(operations) > maxFuzzOperations {
			t.Skip()
		}

		postingCount %= maxFuzzPostings + 1
		documentCount %= maxFuzzDocuments + 1
		documentLengths := make([]uint32, int(documentCount))
		for position := range documentLengths {
			documentLengths[position] = math.MaxUint32
		}
		codec := PostingsCodecRaw
		if vbyte {
			codec = PostingsCodecVByte
		}
		cursor := Cursor{
			input:             bytes.NewReader(data),
			term:              termEntry{postingsBytes: uint64(len(data))},
			codec:             codec,
			documentLengths:   documentLengths,
			postingsRemaining: uint64(postingCount),
		}
		if err := cursor.loadBlock(); err != nil {
			return
		}

		previous, valid := cursor.Current()
		if !valid {
			t.Fatal("successful cursor load has no current posting")
		}
		for position := 0; position+1 < len(operations); position += 2 {
			advance := operations[position]&1 != 0
			target := index.DocumentID(operations[position+1])
			var valid bool
			var err error
			if advance {
				valid, err = cursor.Advance(target)
			} else {
				valid, err = cursor.Next()
			}
			if err != nil {
				return
			}

			current, currentValid := cursor.Current()
			if valid != currentValid {
				t.Fatalf("operation validity = %t, Current validity = %t", valid, currentValid)
			}
			if !valid {
				return
			}
			if current.DocumentID < previous.DocumentID {
				t.Fatalf("cursor moved backward from %d to %d", previous.DocumentID, current.DocumentID)
			}
			if !advance && current.DocumentID == previous.DocumentID {
				t.Fatalf("Next kept document ID %d", current.DocumentID)
			}
			if advance && current.DocumentID < target {
				t.Fatalf("Advance stopped at document %d before its target", current.DocumentID)
			}
			previous = current
		}
	})
}

func FuzzOpenIndex(f *testing.F) {
	names := [...]string{
		MetadataFileName,
		TermsFileName,
		PostingsFileName,
		DocumentLengthsFileName,
		DocumentOffsetsFileName,
		DocumentDataFileName,
	}
	files := make(map[string][]byte, len(names))
	for fileIndex, name := range names {
		files[name] = readGoldenIndexFile(f, name)
		f.Add(uint8(fileIndex), files[name])
	}

	f.Fuzz(func(t *testing.T, fileIndex uint8, data []byte) {
		if len(data) > maxFuzzInputBytes {
			t.Skip()
		}
		selected := names[int(fileIndex)%len(names)]
		opened, err := openIndex("", func(path string) (indexFile, error) {
			name := filepath.Base(path)
			contents := files[name]
			if name == selected {
				contents = data
			}
			return newFuzzIndexFile(contents), nil
		})
		if err != nil {
			return
		}
		if err := opened.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func encodeFuzzPostingList(t testing.TB, codec PostingsCodec, postings []index.Posting) []byte {
	t.Helper()
	position := 0
	next := func() (index.Posting, error) {
		posting := postings[position]
		position++
		return posting, nil
	}

	var encoded bytes.Buffer
	var blockBuffer [postingBlockHeaderBytes + maxVBytePostingPayloadBytes]byte
	var err error
	switch codec {
	case PostingsCodecRaw:
		_, err = writeRawPostingList(&encoded, blockBuffer[:], uint64(len(postings)), next)
	case PostingsCodecVByte:
		_, err = writeVBytePostingList(&encoded, blockBuffer[:], uint64(len(postings)), next)
	default:
		t.Fatalf("unsupported postings codec %d", codec)
	}
	if err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

type fuzzIndexFile struct {
	*bytes.Reader
}

func newFuzzIndexFile(data []byte) *fuzzIndexFile {
	return &fuzzIndexFile{Reader: bytes.NewReader(data)}
}

func (f *fuzzIndexFile) Close() error {
	return nil
}

func (f *fuzzIndexFile) Stat() (fs.FileInfo, error) {
	return fuzzFileInfo(f.Size()), nil
}

type fuzzFileInfo int64

func (f fuzzFileInfo) Name() string       { return "" }
func (f fuzzFileInfo) Size() int64        { return int64(f) }
func (f fuzzFileInfo) Mode() fs.FileMode  { return 0 }
func (f fuzzFileInfo) ModTime() time.Time { return time.Time{} }
func (f fuzzFileInfo) IsDir() bool        { return false }
func (f fuzzFileInfo) Sys() any           { return nil }

func goldenIndexBody(t testing.TB, name string) []byte {
	t.Helper()
	data := readGoldenIndexFile(t, name)
	return data[fileHeaderBytes : len(data)-fileFooterBytes]
}
