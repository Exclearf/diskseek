package indexfile

import (
	"bytes"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

const (
	maxFuzzInputBytes = 4 << 10
	maxFuzzDocuments  = 256
	maxFuzzPostings   = 256
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

func goldenIndexBody(t testing.TB, name string) []byte {
	t.Helper()
	data := readGoldenIndexFile(t, name)
	return data[fileHeaderBytes : len(data)-fileFooterBytes]
}
