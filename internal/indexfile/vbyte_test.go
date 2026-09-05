package indexfile

import (
	"bytes"
	"math"
	"testing"
)

func TestVByteUint32GoldenValues(t *testing.T) {
	tests := []struct {
		value   uint32
		encoded string
	}{
		{0, "\x80"},
		{1, "\x81"},
		{126, "\xfe"},
		{127, "\xff"},
		{128, "\x01\x80"},
		{129, "\x01\x81"},
		{824, "\x06\xb8"},
		{16382, "\x7f\xfe"},
		{16383, "\x7f\xff"},
		{16384, "\x01\x00\x80"},
		{16385, "\x01\x00\x81"},
		{2097150, "\x7f\x7f\xfe"},
		{2097151, "\x7f\x7f\xff"},
		{2097152, "\x01\x00\x00\x80"},
		{2097153, "\x01\x00\x00\x81"},
		{268435454, "\x7f\x7f\x7f\xfe"},
		{268435455, "\x7f\x7f\x7f\xff"},
		{268435456, "\x01\x00\x00\x00\x80"},
		{268435457, "\x01\x00\x00\x00\x81"},
		{math.MaxUint32 - 1, "\x0f\x7f\x7f\x7f\xfe"},
		{math.MaxUint32, "\x0f\x7f\x7f\x7f\xff"},
	}

	for _, test := range tests {
		var encoded [maxVByteUint32Bytes]byte
		encodedBytes := encodeVByteUint32(encoded[:], test.value)
		if !bytes.Equal(encoded[:encodedBytes], []byte(test.encoded)) {
			t.Fatalf("encode %d = % x, want % x", test.value, encoded[:encodedBytes], test.encoded)
		}

		decoded, consumed, err := decodeVByteUint32(encoded[:encodedBytes])
		if err != nil {
			t.Fatal(err)
		}
		if decoded != test.value || consumed != encodedBytes {
			t.Fatalf(
				"decode % x = (%d, %d), want (%d, %d)",
				encoded[:encodedBytes], decoded, consumed, test.value, encodedBytes,
			)
		}
	}
}

func TestDecodeVByteUint32StopsAtTerminator(t *testing.T) {
	value, consumed, err := decodeVByteUint32([]byte{0x06, 0xb8, 0x81})
	if err != nil {
		t.Fatal(err)
	}
	if value != 824 || consumed != 2 {
		t.Fatalf("decode = (%d, %d), want (824, 2)", value, consumed)
	}
}

func TestDecodeVByteUint32RejectsInvalidEncoding(t *testing.T) {
	tests := [][]byte{
		nil,
		{0x01},
		{0x01, 0x00, 0x00, 0x00, 0x00},
		{0x01, 0x00, 0x00, 0x00, 0x00, 0x80},
		{0x10, 0x00, 0x00, 0x00, 0x80},
		{0x00, 0x80},
	}

	for _, encoded := range tests {
		if _, _, err := decodeVByteUint32(encoded); err == nil {
			t.Fatalf("decode % x error = nil", encoded)
		}
	}
}

func FuzzVByteUint32RoundTrip(f *testing.F) {
	for _, value := range []uint32{0, 127, 128, 824, math.MaxUint32} {
		f.Add(value)
	}

	f.Fuzz(func(t *testing.T, value uint32) {
		var encoded [maxVByteUint32Bytes]byte
		encodedBytes := encodeVByteUint32(encoded[:], value)
		decoded, consumed, err := decodeVByteUint32(encoded[:encodedBytes])
		if err != nil {
			t.Fatal(err)
		}
		if decoded != value || consumed != encodedBytes {
			t.Fatalf("round trip = (%d, %d), want (%d, %d)", decoded, consumed, value, encodedBytes)
		}
	})
}

func FuzzDecodeVByteUint32(f *testing.F) {
	f.Add([]byte{0x80})
	f.Add([]byte{0x06, 0xb8})
	f.Add([]byte{0x00, 0x80})

	f.Fuzz(func(t *testing.T, encoded []byte) {
		value, consumed, err := decodeVByteUint32(encoded)
		if err != nil {
			return
		}

		var canonical [maxVByteUint32Bytes]byte
		canonicalBytes := encodeVByteUint32(canonical[:], value)
		if consumed != canonicalBytes || !bytes.Equal(encoded[:consumed], canonical[:canonicalBytes]) {
			t.Fatalf("decode accepted non-canonical prefix % x", encoded[:consumed])
		}
	})
}
