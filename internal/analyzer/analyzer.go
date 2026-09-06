package analyzer

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kljensen/snowball/english"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// ErrInvalidUTF8 reports that analyzer input is not valid UTF-8.
var ErrInvalidUTF8 = errors.New("invalid UTF-8")

// Analyze converts valid UTF-8 text into normalized search tokens.
func Analyze(text string) ([]string, error) {
	if !utf8.ValidString(text) {
		return nil, ErrInvalidUTF8
	}

	// Decompose before folding for canonical caseless matching, then store NFC.
	// See https://www.unicode.org/versions/Unicode17.0.0/core-spec/chapter-3/#G53523
	text = norm.NFD.String(text)
	text = cases.Fold().String(text)
	text = norm.NFC.String(text)

	runes := []rune(text)
	var tokens []string
	var token strings.Builder

	flush := func() {
		if token.Len() == 0 {
			return
		}
		term := token.String()
		if !english.IsStopWord(term) {
			tokens = append(tokens, english.Stem(term, true))
		}
		token.Reset()
	}

	for i, r := range runes {
		switch {
		case isTokenRune(r):
			token.WriteRune(r)
		case isApostrophe(r) && token.Len() > 0 && i+1 < len(runes) && isTokenRune(runes[i+1]):
			token.WriteByte('\'')
		default:
			flush()
		}
	}
	flush()

	return tokens, nil
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func isApostrophe(r rune) bool {
	return r == '\'' || r == '’'
}
