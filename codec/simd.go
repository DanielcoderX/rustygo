package codec

import (
	"encoding/binary"
)

const (
	hi8 = 0x8080808080808080
	lo8 = 0x0101010101010101
)

// HasZeroByte64 returns true if any of the 8 bytes in w is 0x00.
func HasZeroByte64(w uint64) bool {
	return ((w - lo8) & ^w & hi8) != 0
}

// FindQuoteOrEscape locates the index of the first '"' (0x22) or '\' (0x5C) byte in data.
// Returns -1 if neither is found.
func FindQuoteOrEscape(data []byte) int {
	i := 0
	// 8-byte SWAR vector scan loop
	for i+8 <= len(data) {
		w := binary.LittleEndian.Uint64(data[i:])
		xorQuote := w ^ 0x2222222222222222
		xorSlash := w ^ 0x5C5C5C5C5C5C5C5C

		if HasZeroByte64(xorQuote) || HasZeroByte64(xorSlash) {
			break
		}
		i += 8
	}

	// Remaining scalar byte scan
	for ; i < len(data); i++ {
		b := data[i]
		if b == '"' || b == '\\' {
			return i
		}
	}
	return -1
}

// FindByte locates the first occurrence of target byte in data using 64-bit vector scanning.
// Returns -1 if not found.
func FindByte(data []byte, target byte) int {
	rep := uint64(target) * 0x0101010101010101
	i := 0

	for i+8 <= len(data) {
		w := binary.LittleEndian.Uint64(data[i:])
		xor := w ^ rep
		if HasZeroByte64(xor) {
			break
		}
		i += 8
	}

	for ; i < len(data); i++ {
		if data[i] == target {
			return i
		}
	}
	return -1
}

// UnmaskBytesVectorized performs 64-bit word XOR in-place unmasking on payload.
func UnmaskBytesVectorized(payload []byte, mask [4]byte) {
	mask32 := binary.LittleEndian.Uint32(mask[:])
	mask64 := uint64(mask32) | (uint64(mask32) << 32)

	i := 0
	for i+8 <= len(payload) {
		w := binary.LittleEndian.Uint64(payload[i:])
		binary.LittleEndian.PutUint64(payload[i:], w^mask64)
		i += 8
	}

	for ; i < len(payload); i++ {
		payload[i] ^= mask[i%4]
	}
}
