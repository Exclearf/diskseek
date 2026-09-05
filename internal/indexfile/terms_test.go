package indexfile

import (
	"bytes"
	"encoding/binary"
	"testing"
)

var goTermRecord = []byte{
	0x02, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	'g', 'o',
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
		data           []byte
		remainingBytes uint64
		totalDocuments uint64
	}{
		"zero term length":        {zeroTermLength, uint64(len(zeroTermLength)), 2},
		"oversized term length":   {oversizedTermLength, uint64(len(oversizedTermLength)), 2},
		"record crosses body":     {goTermRecord, uint64(len(goTermRecord) - 1), 2},
		"invalid UTF-8":           {invalidUTF8, uint64(len(invalidUTF8)), 2},
		"zero document frequency": {zeroDocumentFrequency, uint64(len(zeroDocumentFrequency)), 2},
		"frequency above corpus":  {goTermRecord, uint64(len(goTermRecord)), 1},
		"zero postings length":    {zeroPostingsLength, uint64(len(zeroPostingsLength)), 2},
		"truncated term":          {goTermRecord[:len(goTermRecord)-1], uint64(len(goTermRecord)), 2},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := readTermRecord(
				bytes.NewReader(test.data), test.remainingBytes, test.totalDocuments,
			); err == nil {
				t.Fatal("readTermRecord returned nil error")
			}
		})
	}
}
