package indexfile

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

var goTermRecord = []byte{
	0x02, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	'g', 'o',
}

var yakTermRecord = []byte{
	0x03, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	'y', 'a', 'k',
}

func TestWriteTermRecordBytes(t *testing.T) {
	var encoded bytes.Buffer
	if err := writeTermRecord(&encoded, termRecord{
		term:              "go",
		documentFrequency: 2,
		postingsBytes:     24,
	}); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(encoded.Bytes(), goTermRecord) {
		t.Fatalf("term record = % x, want % x", encoded.Bytes(), goTermRecord)
	}
}

func TestReadTermRecord(t *testing.T) {
	record, err := readTermRecord(
		bytes.NewReader(goTermRecord), uint64(len(goTermRecord)), 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	want := termRecord{term: "go", documentFrequency: 2, postingsBytes: 24}
	if record != want {
		t.Fatalf("term record = %+v, want %+v", record, want)
	}
}

func TestReadTermRecordRejectsInvalidData(t *testing.T) {
	zeroTermLength := append([]byte(nil), goTermRecord...)
	binary.LittleEndian.PutUint32(zeroTermLength[:4], 0)

	oversizedLength := maxTermBytes + 1
	oversizedTermLength := make([]byte, termRecordHeaderBytes+uint64(oversizedLength))
	binary.LittleEndian.PutUint32(oversizedTermLength[:4], oversizedLength)
	binary.LittleEndian.PutUint64(oversizedTermLength[4:12], 2)
	binary.LittleEndian.PutUint64(oversizedTermLength[12:20], 24)

	invalidUTF8 := append([]byte(nil), goTermRecord...)
	invalidUTF8[20] = 0xff

	zeroDocumentFrequency := append([]byte(nil), goTermRecord...)
	binary.LittleEndian.PutUint64(zeroDocumentFrequency[4:12], 0)

	zeroPostingsLength := append([]byte(nil), goTermRecord...)
	binary.LittleEndian.PutUint64(zeroPostingsLength[12:20], 0)

	tests := map[string]struct {
		data               []byte
		remainingBytes     uint64
		documentsWithTerms uint64
	}{
		"zero term length":                     {zeroTermLength, uint64(len(zeroTermLength)), 2},
		"oversized term length":                {oversizedTermLength, uint64(len(oversizedTermLength)), 2},
		"record crosses body":                  {goTermRecord, uint64(len(goTermRecord) - 1), 2},
		"invalid UTF-8":                        {invalidUTF8, uint64(len(invalidUTF8)), 2},
		"zero document frequency":              {zeroDocumentFrequency, uint64(len(zeroDocumentFrequency)), 2},
		"frequency above documents with terms": {goTermRecord, uint64(len(goTermRecord)), 1},
		"zero postings length":                 {zeroPostingsLength, uint64(len(zeroPostingsLength)), 2},
		"truncated term":                       {goTermRecord[:len(goTermRecord)-1], uint64(len(goTermRecord)), 2},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readTermRecord(
				bytes.NewReader(test.data), test.remainingBytes, test.documentsWithTerms,
			); err == nil {
				t.Fatal("readTermRecord returned nil error")
			}
		})
	}
}

func TestReadTermsDerivesPostingsRanges(t *testing.T) {
	body := append(append([]byte(nil), goTermRecord...), yakTermRecord...)
	terms, err := readTerms(bytes.NewReader(body), uint64(len(body)), 40, 2)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]termEntry{
		"go":  {documentFrequency: 2, postingsOffset: 8, postingsBytes: 24},
		"yak": {documentFrequency: 1, postingsOffset: 32, postingsBytes: 16},
	}
	if len(terms) != len(want) {
		t.Fatalf("term count = %d, want %d", len(terms), len(want))
	}
	for term, wantEntry := range want {
		if terms[term] != wantEntry {
			t.Fatalf("entry for %q = %+v, want %+v", term, terms[term], wantEntry)
		}
	}
}

func TestReadTermsEmpty(t *testing.T) {
	terms, err := readTerms(bytes.NewReader(nil), 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 0 {
		t.Fatalf("term count = %d, want 0", len(terms))
	}
}

func TestReadTermsRejectsInvalidSequence(t *testing.T) {
	duplicate := append(append([]byte(nil), goTermRecord...), goTermRecord...)
	decreasing := append(append([]byte(nil), yakTermRecord...), goTermRecord...)

	wrappedRange := append(append([]byte(nil), goTermRecord...), yakTermRecord...)
	binary.LittleEndian.PutUint64(wrappedRange[12:20], math.MaxUint64)
	binary.LittleEndian.PutUint64(wrappedRange[len(goTermRecord)+12:], 24)

	tests := map[string]struct {
		body          []byte
		postingsBytes uint64
	}{
		"duplicate term":          {duplicate, 48},
		"decreasing term":         {decreasing, 40},
		"wrapped postings ranges": {wrappedRange, 23},
		"unreferenced postings":   {goTermRecord, 25},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readTerms(
				bytes.NewReader(test.body), uint64(len(test.body)), test.postingsBytes, 2,
			); err == nil {
				t.Fatal("readTerms returned nil error")
			}
		})
	}
}

func TestReadTermFile(t *testing.T) {
	data := writeIndexFileTestData(t, termsRole, goTermRecord)
	terms, err := readTermFile(bytes.NewReader(data), int64(len(data)), 24, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := termEntry{documentFrequency: 2, postingsOffset: 8, postingsBytes: 24}
	if terms["go"] != want {
		t.Fatalf("entry for %q = %+v, want %+v", "go", terms["go"], want)
	}
}

func TestReadTermFileValidatesChecksum(t *testing.T) {
	data := writeIndexFileTestData(t, termsRole, goTermRecord)
	data[fileHeaderBytes+termRecordHeaderBytes] = 'h'
	if _, err := readTermFile(bytes.NewReader(data), int64(len(data)), 24, 2); err == nil {
		t.Fatal("readTermFile() error = nil")
	}
}
