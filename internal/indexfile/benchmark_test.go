package indexfile

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

func BenchmarkPostingBlock(b *testing.B) {
	codecs := []struct {
		name  string
		codec PostingsCodec
	}{
		{name: "raw", codec: PostingsCodecRaw},
		{name: "vbyte", codec: PostingsCodecVByte},
	}
	sizes := []struct {
		name  string
		count int
	}{
		{name: "short", count: 8},
		{name: "full", count: postingsPerBlock},
	}

	for _, codec := range codecs {
		for _, size := range sizes {
			fixture := newPostingBlockBenchmarkFixture(b, codec.codec, size.count)
			name := "codec=" + codec.name + "/size=" + size.name
			b.Run(name+"/layer=decode", func(b *testing.B) {
				decoded := make([]index.Posting, size.count)
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.payload)))
				for b.Loop() {
					if err := decodeBenchmarkPayload(codec.codec, fixture.payload, decoded); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(name+"/layer=decode_validate", func(b *testing.B) {
				decoded := make([]index.Posting, size.count)
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.payload)))
				for b.Loop() {
					if err := decodeBenchmarkPayload(codec.codec, fixture.payload, decoded); err != nil {
						b.Fatal(err)
					}
					if err := validatePostingBlock(decoded, fixture.header); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(name+"/layer=load", func(b *testing.B) {
				input := bytes.NewReader(fixture.cursor.data)
				var encoded [maxVBytePostingPayloadBytes]byte
				decoded := make([]index.Posting, size.count)
				b.ReportAllocs()
				b.SetBytes(int64(len(fixture.payload)))
				for b.Loop() {
					header, err := readPostingBlockHeaderAt(
						input,
						fixture.cursor.term,
						fileHeaderBytes,
						size.count,
						uint64(len(fixture.cursor.documentLengths)),
						codec.codec,
						encoded[:],
					)
					if err != nil {
						b.Fatal(err)
					}
					if err := readPostingBlockPayloadAt(
						input,
						fileHeaderBytes,
						header,
						codec.codec,
						fixture.cursor.documentLengths,
						encoded[:header.payloadBytes],
						decoded,
					); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkCursorTraversal(b *testing.B) {
	postings := spacedCursorTestPostings(postingsPerBlock * 64)
	for _, codec := range []struct {
		name  string
		codec PostingsCodec
	}{
		{name: "raw", codec: PostingsCodecRaw},
		{name: "vbyte", codec: PostingsCodecVByte},
	} {
		fixture := newCursorTestFixture(b, postings, codec.codec)
		name := "codec=" + codec.name
		b.Run(name+"/method=next", func(b *testing.B) {
			var calls uint64
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				cursor := fixture.open(b)
				b.StartTimer()
				for {
					valid, err := cursor.Next()
					calls++
					if err != nil {
						b.Fatal(err)
					}
					if !valid {
						break
					}
				}
			}
			reportCursorCalls(b, calls)
		})
		b.Run(name+"/method=advance_near", func(b *testing.B) {
			var calls uint64
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				cursor := fixture.open(b)
				b.StartTimer()
				for {
					posting, _ := cursor.Current()
					valid, err := cursor.Advance(posting.DocumentID + 2)
					calls++
					if err != nil {
						b.Fatal(err)
					}
					if !valid {
						break
					}
				}
			}
			reportCursorCalls(b, calls)
		})
		b.Run(name+"/method=advance_far", func(b *testing.B) {
			const distance = index.DocumentID(2 * postingsPerBlock * 4)
			var calls uint64
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				cursor := fixture.open(b)
				b.StartTimer()
				for {
					posting, _ := cursor.Current()
					valid, err := cursor.Advance(posting.DocumentID + distance)
					calls++
					if err != nil {
						b.Fatal(err)
					}
					if !valid {
						break
					}
				}
			}
			reportCursorCalls(b, calls)
		})
	}
}

type postingBlockBenchmarkFixture struct {
	cursor  cursorTestFixture
	header  postingBlockHeader
	payload []byte
}

func newPostingBlockBenchmarkFixture(
	b *testing.B,
	codec PostingsCodec,
	postingCount int,
) postingBlockBenchmarkFixture {
	b.Helper()
	postings := make([]index.Posting, postingCount)
	documentID := index.DocumentID(100_000)
	for position := range postings {
		if position != 0 {
			documentID += index.DocumentID(position%17 + 1)
		}
		postings[position] = index.Posting{
			DocumentID: documentID,
			Frequency:  uint32(position%7 + 1),
		}
	}

	cursor := newCursorTestFixture(b, postings, codec)
	block := cursor.data[fileHeaderBytes:]
	header, err := decodePostingBlockHeader(block[:postingBlockHeaderBytes], uint64(len(cursor.documentLengths)))
	if err != nil {
		b.Fatal(err)
	}
	return postingBlockBenchmarkFixture{
		cursor:  cursor,
		header:  header,
		payload: block[postingBlockHeaderBytes:],
	}
}

func decodeBenchmarkPayload(codec PostingsCodec, payload []byte, postings []index.Posting) error {
	switch codec {
	case PostingsCodecRaw:
		return decodeRawPostingPayload(payload, postings)
	case PostingsCodecVByte:
		return decodeVBytePostingPayload(payload, postings)
	default:
		return fmt.Errorf("unsupported postings codec %d", codec)
	}
}

func reportCursorCalls(b *testing.B, calls uint64) {
	b.Helper()
	b.ReportMetric(float64(calls)/float64(b.N), "calls/op")
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(calls), "ns/call")
}
