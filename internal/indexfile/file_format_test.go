package indexfile

import (
	"bytes"
	"testing"
)

func TestHeaderBytes(t *testing.T) {
	tests := []struct {
		name string
		role fileRole
		want string
	}{
		{name: "metadata", role: metadataRole, want: "DSKMETA\x01"},
		{name: "terms", role: termsRole, want: "DSKTERM\x01"},
		{name: "postings", role: postingsRole, want: "DSKPOST\x01"},
		{name: "document lengths", role: documentLengthsRole, want: "DSKDLEN\x01"},
		{name: "document offsets", role: documentOffsetsRole, want: "DSKDOFF\x01"},
		{name: "document data", role: documentDataRole, want: "DSKDDAT\x01"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var encoded bytes.Buffer
			if err := writeHeader(&encoded, test.role); err != nil {
				t.Fatal(err)
			}
			if got := encoded.String(); got != test.want {
				t.Fatalf("header = %q, want %q", got, test.want)
			}
			if err := readHeader(bytes.NewReader(encoded.Bytes()), test.role); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadHeaderRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "truncated", data: "DSKTERM"},
		{name: "wrong role", data: "DSKPOST\x01"},
		{name: "wrong version", data: "DSKTERM\x02"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := readHeader(bytes.NewBufferString(test.data), termsRole); err == nil {
				t.Fatal("readHeader() error = nil")
			}
		})
	}
}
