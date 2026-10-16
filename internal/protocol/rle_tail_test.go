package protocol

import "testing"

// TestRLETransmitsTrailingZeros pins the mechanism that makes state channel
// buffer SIZE matter, not just buffer contents.
//
// It is tempting to assume that zeroing the unused tail of a state buffer keeps
// it off the wire — that the encoder will stop at the last meaningful byte. It
// does not. RunLengthEncode walks the whole slice and emits a final zero-run
// pair (0x80, count) for the tail, so the client receives those bytes and
// writes them into its entity struct.
//
// That assumption is exactly what kept the "PS2 cannot move / buff bar fills
// up" bug alive after the tail was zeroed: the serializer stopped writing
// garbage above offset 161, but the channel was still 201 bytes, so 39 zeros
// were still encoded, still sent, and still landed past the end of the client's
// entity layout. The only fix is to make the buffer shorter.
func TestRLETransmitsTrailingZeros(t *testing.T) {
	// A 201-byte buffer shaped like the old object channel: content through
	// offset 161 (ending in the "tsrq" signature), zeros above.
	buf := make([]byte, 201)
	buf[0] = 1
	copy(buf[158:162], "tsrq")

	decoded := runLengthDecode(RunLengthEncode(buf))

	if len(decoded) == 162 {
		t.Fatalf("RunLengthEncode stopped at the last non-zero byte — if that " +
			"were true, buffer size would not matter and the 201 -> 162 fix " +
			"would have been unnecessary. Re-check why the PS2 was overrun.")
	}
	if len(decoded) != 201 {
		t.Fatalf("round trip produced %d bytes, want 201", len(decoded))
	}

	// The 39 trailing zeros really do reach the other side.
	for i := 162; i < 201; i++ {
		if decoded[i] != 0 {
			t.Fatalf("decoded[%d] = %d, want 0", i, decoded[i])
		}
	}

	// And a correctly-sized buffer sends exactly what retail sends.
	short := buf[:162]
	if got := runLengthDecode(RunLengthEncode(short)); len(got) != 162 {
		t.Fatalf("162-byte buffer round-tripped to %d bytes, want 162", len(got))
	}
}
