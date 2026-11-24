package protocol

import (
	"hash/crc32"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
)

// CRCXorConstant is the XOR constant applied after CRC32 calculation.
// This is specific to the EQOA protocol.
const CRCXorConstant uint32 = 0x11f19ed3

// CalculateCRC computes the wire CRC. ChecksumIEEE applies a final complement;
// undo that complement before applying the protocol's XOR.
func CalculateCRC(data []byte) uint32 {
	return ^crc32.ChecksumIEEE(data) ^ CRCXorConstant
}

// ValidateCRC checks if the packet CRC is valid.
// The packet should have the CRC as the last 4 bytes.
func ValidateCRC(packet []byte) bool {
	if len(packet) < 4 {
		return false
	}

	dataLen := len(packet) - 4
	expectedCRC := CalculateCRC(packet[:dataLen])

	// Read CRC from packet (little-endian)
	actualCRC := uint32(packet[dataLen]) |
		uint32(packet[dataLen+1])<<8 |
		uint32(packet[dataLen+2])<<16 |
		uint32(packet[dataLen+3])<<24

	return expectedCRC == actualCRC
}

// AppendCRC appends the CRC32 checksum to a packet.
func AppendCRC(data []byte) []byte {
	crc := CalculateCRC(data)
	return append(data,
		byte(crc),
		byte(crc>>8),
		byte(crc>>16),
		byte(crc>>24),
	)
}

// Encode7BitUInt64 encodes a uint64 as a 7-bit encoded byte slice.
func Encode7BitUInt64(val uint64) []byte {
	if val == 0 {
		return []byte{0}
	}

	var result []byte
	for val > 0 {
		b := byte(val & 0x7F)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		result = append(result, b)
	}
	return result
}

// Encode7BitInt64 encodes an int64 using zigzag encoding then 7-bit encoding.
func Encode7BitInt64(val int64) []byte {
	var uval uint64
	if val < 0 {
		uval = (uint64(^val) << 1) + 1
	} else {
		uval = uint64(val) << 1
	}
	return Encode7BitUInt64(uval)
}

// Decode7BitUInt64 decodes a 7-bit encoded uint64 from a byte slice.
// Returns the value and the number of bytes consumed.
func Decode7BitUInt64(data []byte) (uint64, int) {
	var result uint64
	var shift int
	var i int

	for i < len(data) && shift < 64 {
		b := data[i]
		result |= uint64(b&0x7F) << shift
		i++
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
	}

	return result, i
}

// Decode7BitInt64 decodes a zigzag-encoded 7-bit int64 from a byte slice.
// Returns the value and the number of bytes consumed.
func Decode7BitInt64(data []byte) (int64, int) {
	val, n := Decode7BitUInt64(data)
	result := int64(val >> 1)
	if (val & 1) != 0 {
		result = ^result
	}
	return result, n
}

// Size7BitEncoded returns the number of bytes needed to encode a uint64.
func Size7BitEncoded(val uint64) int {
	if val == 0 {
		return 1
	}
	size := 0
	for val > 0 {
		val >>= 7
		size++
	}
	return size
}

// WriteWideString writes the UTF-16 unit count followed by the little-endian
// units. A supplementary character occupies two units in the game's stream.
func WriteWideString(w *buffer.Writer, text string) error {
	units := utf16.Encode([]rune(text))
	if err := w.WriteUint32(uint32(len(units))); err != nil {
		return err
	}
	for _, unit := range units {
		if err := w.WriteUint16(unit); err != nil {
			return err
		}
	}
	return nil
}
