package corpus

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestTSVReader(t *testing.T) {
	reader := NewTSVReader(strings.NewReader("1\tfirst\n1\tsecond\twith tab\n2\t\n3\tlast"))
	want := []Record{
		{ExternalID: "1", Text: "first"},
		{ExternalID: "1", Text: "second\twith tab"},
		{ExternalID: "2", Text: ""},
		{ExternalID: "3", Text: "last"},
	}

	for i, expected := range want {
		got, err := reader.Next()
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if got != expected {
			t.Fatalf("record %d = %#v, want %#v", i, got, expected)
		}
	}

	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("after final record error = %v, want EOF", err)
	}
}

func TestTSVReaderRejectsInvalidRecords(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  error
	}{
		{name: "missing tab", input: "broken\n", want: ErrMalformedRecord},
		{name: "empty external ID", input: "\ttext\n", want: ErrEmptyExternalID},
		{name: "invalid UTF-8", input: "1\t" + string([]byte{0xff}) + "\n", want: ErrInvalidUTF8},
		{name: "external ID too large", input: strings.Repeat("a", MaxExternalIDBytes+1) + "\ttext\n", want: ErrExternalIDTooLarge},
		{name: "record too large", input: "1\t" + strings.Repeat("a", MaxRecordBytes-1) + "\n", want: ErrRecordTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTSVReader(strings.NewReader(tt.input)).Next()
			if !errors.Is(err, tt.want) {
				t.Fatalf("Next() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestTSVReaderAcceptsMaximumRecordSize(t *testing.T) {
	text := strings.Repeat("a", MaxRecordBytes-2)

	record, err := NewTSVReader(strings.NewReader("1\t" + text + "\r\n")).Next()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Text) != len(text) {
		t.Fatalf("text length = %d, want %d", len(record.Text), len(text))
	}
}
