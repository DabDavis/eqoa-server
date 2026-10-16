package handlers

import (
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
)

// TestMemoryDumpTailMatchesClientReadSequence reads the tail back exactly the
// way the PS2 client's MemoryDump handler (func_00BD0630) reads it, and fails
// if the client would run off the end of the buffer.
//
// This is the regression the tail was written for: the old tail was 8 bytes
// short of the two CM stat arrays, the client's stream hit end-of-buffer
// mid-array, the whole MemoryDump was rejected, and picking a character
// dropped the PS2 back to server select ~440ms later. Nothing about the
// packet delivery looked wrong, so only a byte-level read-back catches it.
func TestMemoryDumpTailMatchesClientReadSequence(t *testing.T) {
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	player := &entity.Player{Speed: 8.8333}
	writeMemoryDumpTail(w, player)

	written := w.Position()
	r := buffer.NewReader(buf[:written])

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("client read of %s failed at offset %d (%d written): %v",
				what, r.Position(), written, err)
		}
	}
	zigzag := func(what string) int64 {
		t.Helper()
		v, err := r.Read7BitEncodedInt64()
		must(what, err)
		return v
	}
	raw32 := func(what string) uint32 {
		t.Helper()
		v, err := r.ReadUint32()
		must(what, err)
		return v
	}

	// func_00BBCD10 spell list, func_00BBD0A8 buff list.
	if n := zigzag("spellCount"); n != 0 {
		t.Errorf("spellCount = %d, want 0 (retail carries abilities in the skill table)", n)
	}
	if n := zigzag("buffCount"); n != 0 {
		t.Errorf("buffCount = %d, want 0", n)
	}

	// Equipment trailing flag + pending-loot flag.
	flag, err := r.ReadByte()
	must("equipment flag", err)
	if flag != 1 {
		t.Errorf("equipment flag = %d, want 1 (every capture sends 1)", flag)
	}
	loot, err := r.ReadByte()
	must("hasPendingLoot", err)
	if loot != 0 {
		t.Errorf("hasPendingLoot = %d, want 0", loot)
	}

	// func_00BDCDC0 skill table. A non-zero count here means rows follow, and
	// the reader below would be reading them as the speed/hotbar block.
	if n := zigzag("skillCount"); n != 0 {
		t.Fatalf("skillCount = %d, want 0 — rows must be in func_00BDCBE0 format", n)
	}

	speed, err := r.ReadFloat32()
	must("run speed", err)
	if speed != player.Speed {
		t.Errorf("run speed = %v, want %v", speed, player.Speed)
	}
	raw32("unknown after speed")

	// func_00BFD178 weapon hotbar: 6 rows of 4 raw uint32.
	for i, want := range weaponHotbarRows {
		for j, wantVal := range want {
			if got := raw32("weapon hotbar"); got != wantVal {
				t.Errorf("hotbar[%d][%d] = %d, want %d", i, j, got, wantVal)
			}
		}
	}

	// func_00BF71A0 CM block: raw4, 4 zigzag, raw4, then 29 + 7 zigzag.
	raw32("cm +0x41C")
	for _, off := range []string{"+0x0C", "+0x10", "+0x14", "+0x08"} {
		zigzag("cm " + off)
	}
	raw32("cm +0xA8")
	for i := 0; i < 29; i++ {
		zigzag("cm stat array 1")
	}
	for i := 0; i < 7; i++ {
		zigzag("cm stat array 2")
	}

	// Everything the client is known to read is now consumed. What is left is
	// the trailing run every captured retail dump carries past that point.
	if got := r.Remaining(); got != memoryDumpTrailerLen {
		t.Errorf("trailer = %d bytes, want %d (retail dumps leave exactly this much unread)",
			got, memoryDumpTrailerLen)
	}
	for i, b := range buf[r.Position():written] {
		if b != 0 {
			t.Errorf("trailer byte %d = 0x%02X, want 0x00", i, b)
			break
		}
	}
}
