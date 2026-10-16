package protocol

import (
	"bytes"
	"testing"
)

func TestCalculateCRC(t *testing.T) {
	// Test that CRC is deterministic
	data := []byte{0x01, 0x02, 0x03, 0x04}
	crc1 := CalculateCRC(data)
	crc2 := CalculateCRC(data)

	if crc1 != crc2 {
		t.Errorf("CRC should be deterministic: got 0x%08X and 0x%08X", crc1, crc2)
	}

	// Test that different data produces different CRC
	data2 := []byte{0x01, 0x02, 0x03, 0x05}
	crc3 := CalculateCRC(data2)

	if crc1 == crc3 {
		t.Errorf("Different data should produce different CRC")
	}

	// Test empty data
	emptyCRC := CalculateCRC([]byte{})
	if emptyCRC == 0 {
		t.Errorf("Empty data CRC should not be 0")
	}
}

func TestValidateCRC(t *testing.T) {
	// Create test packet with valid CRC
	data := []byte{0x01, 0x02, 0x03, 0x04}
	crc := CalculateCRC(data)
	packet := append(data,
		byte(crc),
		byte(crc>>8),
		byte(crc>>16),
		byte(crc>>24),
	)

	if !ValidateCRC(packet) {
		t.Error("expected valid CRC")
	}

	// Corrupt the packet
	packet[0] ^= 0xFF
	if ValidateCRC(packet) {
		t.Error("expected invalid CRC after corruption")
	}
}

func TestAppendCRC(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	result := AppendCRC(data)

	if len(result) != len(data)+4 {
		t.Errorf("expected length %d, got %d", len(data)+4, len(result))
	}

	if !ValidateCRC(result) {
		t.Error("appended CRC should validate")
	}
}

func TestEncode7BitUInt64(t *testing.T) {
	tests := []struct {
		value    uint64
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7F}},
		{128, []byte{0x80, 0x01}},
		{255, []byte{0xFF, 0x01}},
		{256, []byte{0x80, 0x02}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x80, 0x80, 0x01}},
	}

	for _, tt := range tests {
		result := Encode7BitUInt64(tt.value)
		if !bytes.Equal(result, tt.expected) {
			t.Errorf("Encode7BitUInt64(%d): expected %v, got %v",
				tt.value, tt.expected, result)
		}
	}
}

func TestDecode7BitUInt64(t *testing.T) {
	tests := []struct {
		input    []byte
		expected uint64
		consumed int
	}{
		{[]byte{0x00}, 0, 1},
		{[]byte{0x01}, 1, 1},
		{[]byte{0x7F}, 127, 1},
		{[]byte{0x80, 0x01}, 128, 2},
		{[]byte{0xFF, 0x01}, 255, 2},
		{[]byte{0x80, 0x02}, 256, 2},
		{[]byte{0xFF, 0x7F}, 16383, 2},
		{[]byte{0x80, 0x80, 0x01}, 16384, 3},
	}

	for _, tt := range tests {
		result, consumed := Decode7BitUInt64(tt.input)
		if result != tt.expected {
			t.Errorf("Decode7BitUInt64(%v): expected value %d, got %d",
				tt.input, tt.expected, result)
		}
		if consumed != tt.consumed {
			t.Errorf("Decode7BitUInt64(%v): expected consumed %d, got %d",
				tt.input, tt.consumed, consumed)
		}
	}
}

func TestEncode7BitInt64(t *testing.T) {
	tests := []struct {
		value    int64
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x02}},
		{-1, []byte{0x01}},
		{2, []byte{0x04}},
		{-2, []byte{0x03}},
		{63, []byte{0x7E}},
		{-64, []byte{0x7F}},
	}

	for _, tt := range tests {
		result := Encode7BitInt64(tt.value)
		if !bytes.Equal(result, tt.expected) {
			t.Errorf("Encode7BitInt64(%d): expected %v, got %v",
				tt.value, tt.expected, result)
		}
	}
}

func TestDecode7BitInt64(t *testing.T) {
	tests := []struct {
		input    []byte
		expected int64
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x02}, 1},
		{[]byte{0x01}, -1},
		{[]byte{0x04}, 2},
		{[]byte{0x03}, -2},
		{[]byte{0x7E}, 63},
		{[]byte{0x7F}, -64},
	}

	for _, tt := range tests {
		result, _ := Decode7BitInt64(tt.input)
		if result != tt.expected {
			t.Errorf("Decode7BitInt64(%v): expected %d, got %d",
				tt.input, tt.expected, result)
		}
	}
}

func Test7BitRoundtrip(t *testing.T) {
	// Test unsigned roundtrip
	unsignedTests := []uint64{
		0, 1, 127, 128, 255, 256, 16383, 16384,
		65535, 1000000, 0xFFFFFFFF,
	}

	for _, val := range unsignedTests {
		encoded := Encode7BitUInt64(val)
		decoded, _ := Decode7BitUInt64(encoded)
		if decoded != val {
			t.Errorf("unsigned roundtrip failed: %d -> %d", val, decoded)
		}
	}

	// Test signed roundtrip
	signedTests := []int64{
		0, 1, -1, 63, -64, 127, -128, 1000, -1000,
		0x7FFFFFFF, -0x80000000,
	}

	for _, val := range signedTests {
		encoded := Encode7BitInt64(val)
		decoded, _ := Decode7BitInt64(encoded)
		if decoded != val {
			t.Errorf("signed roundtrip failed: %d -> %d", val, decoded)
		}
	}
}

func TestSize7BitEncoded(t *testing.T) {
	tests := []struct {
		value    uint64
		expected int
	}{
		{0, 1},
		{1, 1},
		{127, 1},
		{128, 2},
		{16383, 2},
		{16384, 3},
	}

	for _, tt := range tests {
		result := Size7BitEncoded(tt.value)
		if result != tt.expected {
			t.Errorf("Size7BitEncoded(%d): expected %d, got %d",
				tt.value, tt.expected, result)
		}
	}
}

func BenchmarkCalculateCRC(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateCRC(data)
	}
}

func BenchmarkEncode7BitUInt64(b *testing.B) {
	val := uint64(0x12345678)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode7BitUInt64(val)
	}
}

func BenchmarkDecode7BitUInt64(b *testing.B) {
	data := []byte{0x80, 0x80, 0x80, 0x80, 0x01}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode7BitUInt64(data)
	}
}
