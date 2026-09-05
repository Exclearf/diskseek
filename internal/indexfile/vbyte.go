package indexfile

import (
	"errors"
	"math"
)

const maxVByteUint32Bytes = 5

var errInvalidVByteUint32 = errors.New("invalid variable-byte uint32")

func encodeVByteUint32(destination []byte, value uint32) int {
	shift := 0
	for remaining := value >> 7; remaining != 0; remaining >>= 7 {
		shift += 7
	}

	position := 0
	for shift > 0 {
		destination[position] = byte(value>>shift) & 0x7f
		position++
		shift -= 7
	}
	destination[position] = byte(value&0x7f) | 0x80
	return position + 1
}

func decodeVByteUint32(encoded []byte) (uint32, int, error) {
	var value uint32
	for position := 0; position < len(encoded) && position < maxVByteUint32Bytes; position++ {
		current := encoded[position]
		payload := uint32(current & 0x7f)
		if position == 0 && current == 0 {
			return 0, 0, errInvalidVByteUint32
		}

		next := uint64(value)<<7 | uint64(payload)
		if next > math.MaxUint32 {
			return 0, 0, errInvalidVByteUint32
		}
		value = uint32(next)
		if current&0x80 != 0 {
			return value, position + 1, nil
		}
	}
	return 0, 0, errInvalidVByteUint32
}
