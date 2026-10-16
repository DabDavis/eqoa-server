package handlers

import (
	"bytes"
	"testing"
)

// The shard list's count is ZIGZAG: CLIENT's VIShardInfo::Read (0x00BFC208)
// takes it with ReadSigned. The default list is one shard, which is the byte
// 0x02, the same byte the old two-entry list sent raw (and a PS2 read as one).
func TestServerListCountIsZigzag(t *testing.T) {
	h := NewServerHandler("192.0.2.7")
	sess := testSession()
	h.SendServerList(sess)

	msgs := sess.Queue().PeekPending()
	if len(msgs) != 1 {
		t.Fatalf("%d messages queued, want 1", len(msgs))
	}
	d := msgs[0].Data()
	if !bytes.HasPrefix(d, []byte{0xB3, 0x07, 0x02}) {
		t.Fatalf("server list starts % x, want b3 07 02 (opcode, zigzag 1)", d[:3])
	}
	// One entry: u32 length + 11 UTF-16 chars, recommended, endpoint, port,
	// IP and language, and nothing after it.
	if want := 3 + 4 + 2*len("Marr's Fist") + 1 + 2 + 2 + 4 + 1; len(d) != want {
		t.Fatalf("server list is %d bytes, want %d (exactly one entry)", len(d), want)
	}
}
