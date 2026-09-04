package indexfile

import (
	"math"
	"testing"
)

func TestUint64Arithmetic(t *testing.T) {
	tests := []struct {
		name      string
		operation func() (uint64, error)
		want      uint64
		wantError bool
	}{
		{name: "add", operation: func() (uint64, error) { return addUint64(40, 2) }, want: 42},
		{name: "add maximum", operation: func() (uint64, error) { return addUint64(math.MaxUint64, 0) }, want: math.MaxUint64},
		{name: "add overflow", operation: func() (uint64, error) { return addUint64(math.MaxUint64, 1) }, wantError: true},
		{name: "subtract", operation: func() (uint64, error) { return subtractUint64(7, 3) }, want: 4},
		{name: "subtract zero", operation: func() (uint64, error) { return subtractUint64(0, 0) }},
		{name: "subtract underflow", operation: func() (uint64, error) { return subtractUint64(0, 1) }, wantError: true},
		{name: "multiply", operation: func() (uint64, error) { return multiplyUint64(6, 7) }, want: 42},
		{name: "multiply maximum", operation: func() (uint64, error) { return multiplyUint64(math.MaxUint64, 1) }, want: math.MaxUint64},
		{name: "multiply by zero", operation: func() (uint64, error) { return multiplyUint64(math.MaxUint64, 0) }},
		{name: "multiply overflow", operation: func() (uint64, error) { return multiplyUint64(math.MaxUint64, 2) }, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.operation()
			if test.wantError {
				if err == nil {
					t.Fatal("error = nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("result = %d, want %d", got, test.want)
			}
		})
	}
}

func TestUint64Conversions(t *testing.T) {
	int64Value, err := uint64ToInt64(42)
	if err != nil || int64Value != 42 {
		t.Fatalf("uint64ToInt64(42) = %d, %v", int64Value, err)
	}

	int64Value, err = uint64ToInt64(math.MaxInt64)
	if err != nil {
		t.Fatal(err)
	}
	if int64Value != math.MaxInt64 {
		t.Fatalf("int64 value = %d, want %d", int64Value, int64(math.MaxInt64))
	}
	if _, err := uint64ToInt64(uint64(math.MaxInt64) + 1); err == nil {
		t.Fatal("uint64ToInt64() error = nil")
	}

	maxInt := uint64(math.MaxInt)
	intValue, err := uint64ToInt(42)
	if err != nil || intValue != 42 {
		t.Fatalf("uint64ToInt(42) = %d, %v", intValue, err)
	}

	intValue, err = uint64ToInt(maxInt)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(intValue) != maxInt {
		t.Fatalf("int value = %d, want %d", intValue, maxInt)
	}
	if _, err := uint64ToInt(maxInt + 1); err == nil {
		t.Fatal("uint64ToInt() error = nil")
	}
}
