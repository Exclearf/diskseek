package analyzer

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "letters and numbers", text: "Go, GO! 2026", want: []string{"go", "go", "2026"}},
		{name: "precomposed accent", text: "CAFÉ", want: []string{"café"}},
		{name: "decomposed accent", text: "cafe\u0301", want: []string{"café"}},
		{name: "Unicode case fold", text: "Straße", want: []string{"strass"}},
		{name: "apostrophes", text: "'Cat' ’Dog’ Can't rock’n’jazz", want: []string{"cat", "dog", "can't", "rock'n'jazz"}},
		{name: "Porter2 and stopwords", text: "The Engine's engines trees", want: []string{"engin", "engin", "tree"}},
		{name: "mixed scripts", text: "Go 東京", want: []string{"go", "東京"}},
		{name: "empty", text: "", want: nil},
		{name: "separators only", text: "---", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Analyze(tt.text)
			if err != nil {
				t.Fatalf("Analyze(%q): %v", tt.text, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Analyze(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestAnalyzeRejectsInvalidUTF8(t *testing.T) {
	_, err := Analyze(string([]byte{0xff}))
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("Analyze(invalid UTF-8) error = %v, want %v", err, ErrInvalidUTF8)
	}
}

func TestAnalyzeDoesNotSplitLongToken(t *testing.T) {
	text := strings.Repeat("a", 300)

	got, err := Analyze(text)
	if err != nil {
		t.Fatalf("Analyze(long token): %v", err)
	}
	if len(got) != 1 || got[0] != text {
		t.Fatalf("Analyze(long token) = %q, want one unchanged token", got)
	}
}
