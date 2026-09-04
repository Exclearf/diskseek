package search

import (
	"slices"
	"testing"
)

func TestPrepareQuery(t *testing.T) {
	got, err := prepareQuery("Straße ALPHA strasse")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"alpha", "strasse"}
	if !slices.Equal(got, want) {
		t.Fatalf("prepareQuery() = %q, want %q", got, want)
	}
}
