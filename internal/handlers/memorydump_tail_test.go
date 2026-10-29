package handlers

import (
	"math"
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
	player := &entity.Player{Speed: 8.8333, Level: 60, Mastery: entity.Mastery{Points: 12, Spent: 765, Percent: 100}}
	// the masteries it bought give STR +1 and its cap +15 (Brawn 1's)
	player.CMEffects[0], player.CMEffects[7] = 1, 15
	// a character with no tradeskill rows (craft.go)
	writeMemoryDumpTail(w, player, nil, craftSlotsDefault())

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
	if tok := raw32("the tradeskill token"); tok != 0 {
		t.Errorf("tradeskill token = %d, want Sandstorm's 0", tok)
	}

	// ReadSlots (0x00BFD178): the six tradeskill slots, 4 raw uint32 each,
	// for a character with no rows exactly what every capture carries
	// (Sandstorm's, sent to everyone until 2026-10-29 as a "weapon hotbar")
	sandstorm := [6][4]uint32{{99, 1, 0, 98}, {99, 1, 0, 98}, {99, 1, 0, 98}, {99, 1, 0, 98},
		{99, 99, 99, 98}, {99, 99, 99, 98}}
	for i, want := range sandstorm {
		for j, wantVal := range want {
			if got := raw32("tradeskill slot"); got != wantVal {
				t.Errorf("slot[%d][%d] = %d, want %d", i, j, got, wantVal)
			}
		}
	}

	// func_00BF71A0 CM block (ReadState): the token raw; the unspent (+0x0C),
	// spent (+0x10), +0x14 and the share (+0x08) zigzag; the speed raw; the
	// 29 stats the masteries add and the 7 stat caps they raise, zigzag.
	raw32("cm +0x41C")
	for _, f := range []struct {
		off  string
		want int64
	}{{"+0x0C unspent", 12}, {"+0x10 spent", 765}, {"+0x14", 0}, {"+0x08 share", 100}} {
		if got := zigzag("cm " + f.off); got != f.want {
			t.Errorf("cm %s = %d, want %d", f.off, got, f.want)
		}
	}
	if got := math.Float32frombits(raw32("cm speed")); got != player.SpeedNow() {
		t.Errorf("cm speed = %v, want the run speed %v", got, player.SpeedNow())
	}
	for i := 0; i < 29; i++ {
		if got := zigzag("cm stat array"); got != int64(player.CMEffects[i]) {
			t.Errorf("cm stat %d = %d, want %d (what the masteries bought add)", i, got, player.CMEffects[i])
		}
	}
	// the caps they raise: STR's by Brawn 1's 15, the rest by nothing. Until
	// masteries were sold every character was sent the general ones its level
	// allowed (150 each at 60), bought or not.
	for i := 0; i < 7; i++ {
		want := int64(0)
		if i == 0 {
			want = 15
		}
		if got := zigzag("cm stat caps raised"); got != want {
			t.Errorf("cap raise %d = %d, want %d", i, got, want)
		}
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
