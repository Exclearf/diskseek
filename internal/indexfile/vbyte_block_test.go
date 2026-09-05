package indexfile

import (
	"bytes"
	"math"
	"slices"
	"testing"

	"github.com/Exclearf/diskseek/internal/index"
)

const vBytePostingPayloadFixture = "\x80\x81" +
	"\x81\x83" +
	"\x01\x80\x82" +
	"\x05\xb7\x81"

func TestVBytePostingPayloadBytes(t *testing.T) {
	want := []index.Posting{
		{DocumentID: 0, Frequency: 1},
		{DocumentID: 1, Frequency: 3},
		{DocumentID: 129, Frequency: 2},
		{DocumentID: 824, Frequency: 1},
	}

	var encoded [maxVBytePostingPayloadBytes]byte
	encodedBytes, err := encodeVBytePostingPayload(encoded[:], want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded[:encodedBytes], []byte(vBytePostingPayloadFixture)) {
		t.Fatalf("payload = % x, want % x", encoded[:encodedBytes], vBytePostingPayloadFixture)
	}

	got := make([]index.Posting, len(want))
	if err := decodeVBytePostingPayload(encoded[:encodedBytes], got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("postings = %+v, want %+v", got, want)
	}
}

func TestVBytePostingPayloadCounts(t *testing.T) {
	for _, postingCount := range []int{1, 127, 128} {
		want := make([]index.Posting, postingCount)
		for position := range want {
			want[position] = index.Posting{
				DocumentID: index.DocumentID(position * 129),
				Frequency:  uint32(position%3 + 1),
			}
		}

		var encoded [maxVBytePostingPayloadBytes]byte
		encodedBytes, err := encodeVBytePostingPayload(encoded[:], want)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]index.Posting, postingCount)
		if err := decodeVBytePostingPayload(encoded[:encodedBytes], got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%d postings did not round-trip", postingCount)
		}
	}

	if _, err := encodeVBytePostingPayload(make([]byte, maxVBytePostingPayloadBytes), nil); err == nil {
		t.Fatal("encode empty payload error = nil")
	}
	if _, err := encodeVBytePostingPayload(
		make([]byte, maxVBytePostingPayloadBytes+2*maxVByteUint32Bytes),
		make([]index.Posting, postingsPerBlock+1),
	); err == nil {
		t.Fatal("encode oversized payload error = nil")
	}
}

func TestEncodeVBytePostingPayloadRejectsInvalidPostings(t *testing.T) {
	tests := [][]index.Posting{
		{{DocumentID: 0, Frequency: 0}},
		{{DocumentID: 1, Frequency: 1}, {DocumentID: 1, Frequency: 1}},
		{{DocumentID: 1, Frequency: 1}, {DocumentID: 0, Frequency: 1}},
	}

	for _, postings := range tests {
		var encoded [maxVBytePostingPayloadBytes]byte
		if _, err := encodeVBytePostingPayload(encoded[:], postings); err == nil {
			t.Fatalf("encode %+v error = nil", postings)
		}
	}
}

func TestDecodeVBytePostingPayloadRejectsInvalidData(t *testing.T) {
	zeroFrequency := []byte(vBytePostingPayloadFixture)
	zeroFrequency[1] = 0x80

	zeroGap := []byte(vBytePostingPayloadFixture)
	zeroGap[2] = 0x80

	overflow := []byte{
		0x0f, 0x7f, 0x7f, 0x7f, 0xff, 0x81,
		0x81, 0x81,
	}

	tests := []struct {
		payload      []byte
		postingCount int
	}{
		{payload: nil, postingCount: 1},
		{payload: []byte(vBytePostingPayloadFixture[:len(vBytePostingPayloadFixture)-1]), postingCount: 4},
		{payload: append([]byte(vBytePostingPayloadFixture), 0x81), postingCount: 4},
		{payload: zeroFrequency, postingCount: 4},
		{payload: zeroGap, postingCount: 4},
		{payload: overflow, postingCount: 2},
	}

	for _, test := range tests {
		postings := make([]index.Posting, test.postingCount)
		if err := decodeVBytePostingPayload(test.payload, postings); err == nil {
			t.Fatalf("decode % x error = nil", test.payload)
		}
	}

	tooMany := make([]index.Posting, postingsPerBlock+1)
	if err := decodeVBytePostingPayload(make([]byte, len(tooMany)*2), tooMany); err == nil {
		t.Fatal("decode oversized payload error = nil")
	}
}

func FuzzVBytePostingPayloadRoundTrip(f *testing.F) {
	f.Add(uint32(0), uint32(1), uint32(1))
	f.Add(uint32(129), uint32(695), uint32(3))

	f.Fuzz(func(t *testing.T, firstDocumentID, gap, frequency uint32) {
		if gap == 0 || uint64(firstDocumentID)+uint64(gap) > math.MaxUint32 || frequency == 0 {
			return
		}
		want := []index.Posting{
			{DocumentID: index.DocumentID(firstDocumentID), Frequency: frequency},
			{DocumentID: index.DocumentID(firstDocumentID + gap), Frequency: frequency},
		}

		var encoded [maxVBytePostingPayloadBytes]byte
		encodedBytes, err := encodeVBytePostingPayload(encoded[:], want)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]index.Posting, len(want))
		if err := decodeVBytePostingPayload(encoded[:encodedBytes], got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("postings = %+v, want %+v", got, want)
		}
	})
}
