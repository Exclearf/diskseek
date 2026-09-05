package search

import (
	"cmp"

	"github.com/Exclearf/diskseek/internal/index"
)

type result struct {
	DocumentID index.DocumentID
	Score      float64
}

func compareResults(left, right result) int {
	if left.Score > right.Score {
		return -1
	}
	if left.Score < right.Score {
		return 1
	}
	return cmp.Compare(left.DocumentID, right.DocumentID)
}

func compareWorstResults(left, right result) int {
	return compareResults(right, left)
}
