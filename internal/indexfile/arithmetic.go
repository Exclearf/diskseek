package indexfile

import (
	"errors"
	"math"
	"math/bits"
)

func addUint64(left, right uint64) (uint64, error) {
	sum, carry := bits.Add64(left, right, 0)
	if carry != 0 {
		return 0, errors.New("uint64 addition overflows")
	}
	return sum, nil
}

func subtractUint64(left, right uint64) (uint64, error) {
	difference, borrow := bits.Sub64(left, right, 0)
	if borrow != 0 {
		return 0, errors.New("uint64 subtraction underflows")
	}
	return difference, nil
}

func multiplyUint64(left, right uint64) (uint64, error) {
	high, product := bits.Mul64(left, right)
	if high != 0 {
		return 0, errors.New("uint64 multiplication overflows")
	}
	return product, nil
}

func uint64ToInt64(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, errors.New("uint64 value does not fit in int64")
	}
	return int64(value), nil
}

func uint64ToInt(value uint64) (int, error) {
	maxInt := uint64(math.MaxInt)
	if value > maxInt {
		return 0, errors.New("uint64 value does not fit in int")
	}
	return int(value), nil
}
