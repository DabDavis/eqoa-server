package handlers

import (
	"encoding/binary"
	"unicode/utf16"

	"github.com/eqoa/server/internal/protocol"
)

// --- reading a client message ----------------------------------------------------

// clientReader reads a message the way CLIENT's VIMemStream writes one: raw
// little-endian words, seven-bit zigzag ints, and counted strings, from the
// payload after the opcode (Data()). Any short read sets err.
type clientReader struct {
	b   []byte
	p   int
	err bool
}

func newClientReader(msg *protocol.Message) *clientReader {
	return &clientReader{b: msg.Data()}
}

func (r *clientReader) take(n int) []byte {
	if r.err || n < 0 || len(r.b)-r.p < n {
		r.err = true
		return nil
	}
	v := r.b[r.p : r.p+n]
	r.p += n
	return v
}

func (r *clientReader) u8() byte {
	if v := r.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (r *clientReader) u32() uint32 {
	if v := r.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}

// signed is VIMemStream::WriteSigned's zigzag read back.
func (r *clientReader) signed() int32 {
	var u uint32
	for shift := uint(0); ; shift += 7 {
		b := r.u8()
		if r.err {
			return 0
		}
		if shift < 32 {
			u |= uint32(b&0x7F) << shift
		}
		if b&0x80 == 0 {
			break
		}
	}
	return int32(u>>1) ^ -int32(u&1)
}

// str8 is a u32 byte count and the bytes (WriteString(const char *)), cut at
// a NUL; longer than maxBytes-1 reads as "" the way CLIENT keeps it.
func (r *clientReader) str8(maxBytes int) string {
	n := int(r.u32())
	b := r.take(n)
	if b == nil || n+1 > maxBytes {
		return ""
	}
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	return string(b)
}

// str16 is a u32 count of UTF-16 units and the units (WriteString(const
// unsigned short *)), cut at a NUL.
func (r *clientReader) str16(maxUnits int) string {
	n := int(r.u32())
	b := r.take(2 * n)
	if b == nil || n > maxUnits {
		return ""
	}
	u := make([]uint16, 0, n)
	for i := 0; i+1 < len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}
