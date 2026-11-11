package entity

import (
	"encoding/binary"
	"testing"
)

// A player's hidden parts go out as nothing worn on the entity channel (the
// held item without its model, its attack type kept; the armour set and its
// tint 0; the robe none), and every other part as worn; nothing of the player
// changes. With none hidden the channel is byte for byte as before.
func TestHiddenLooksGoOutBare(t *testing.T) {
	p := geared(0)
	want := SerializePlayerUpdate(7, p)

	p.HiddenParts = HideChest | HideHelm | HidePrimary | HideRobe
	b := SerializePlayerUpdate(7, p)
	if binary.LittleEndian.Uint32(b[67:71]) != 0 || b[91] != 1 {
		t.Errorf("the hidden main hand: % x, type %d", b[67:71], b[91])
	}
	if b[93] != 0 || b[99] != 0 || b[98] != 0 || b[104] != 0 {
		t.Errorf("the hidden chest and helm: sets %d %d, tints %d %d", b[93], b[98], b[99], b[104])
	}
	if b[109] != 0xFF || b[110] != 0 {
		t.Errorf("the hidden robe: %d tint %d", b[109], b[110])
	}
	for _, i := range []int{71, 75, 92, 94, 95, 96, 97, 100, 101, 102, 103} {
		if b[i] != want[i] {
			t.Errorf("byte %d, not hidden: %d, want %d", i, b[i], want[i])
		}
	}
	for i := range b {
		if (i < 67 || i > 110) && b[i] != want[i] {
			t.Errorf("byte %d outside the look changed: %d, want %d", i, b[i], want[i])
		}
	}
	if p.Primary == 0 || p.Chest == 0 || !p.RobeWorn {
		t.Error("the player's own gear is untouched")
	}

	p.HiddenParts = HideSecondary | HideShield | HideBracer | HideGloves | HideLegs | HideBoots
	b = SerializePlayerUpdate(7, p)
	if binary.LittleEndian.Uint32(b[71:75]) != 0 || binary.LittleEndian.Uint32(b[75:79]) != 0 || b[92] != 2 {
		t.Errorf("the hidden off hand and shield: % x, type %d", b[71:79], b[92])
	}
	for i := 94; i < 98; i++ {
		if b[i] != 0 || b[i+6] != 0 {
			t.Errorf("hidden armour slot %d: set %d tint %d", i-93, b[i], b[i+6])
		}
	}
	if binary.LittleEndian.Uint32(b[67:71]) == 0 || b[93] == 0 || b[109] == 0xFF {
		t.Error("the parts not hidden this time show")
	}
}

// The character list's 32 gear bytes go bare the same way, part by part.
func TestHiddenLooksInTheCharacterList(t *testing.T) {
	g := make([]byte, 32)
	for i := range g {
		g[i] = 7
	}
	g[17], g[18] = 1, 2
	HideCharacterParts(g, 0)
	for i := range g {
		if want := map[int]byte{17: 1, 18: 2}[i]; want == 0 && g[i] != 7 {
			t.Fatalf("nothing hidden changed byte %d", i)
		}
	}
	HideCharacterParts(g, HideRobe|HidePrimary|HideShield|HideHelm|HideChest)
	if binary.LittleEndian.Uint32(g[0:4]) != 0xFFFFFFFF || g[4] != 0 {
		t.Errorf("the robe: % x", g[0:5])
	}
	if binary.LittleEndian.Uint32(g[5:9]) != 0 || binary.LittleEndian.Uint32(g[13:17]) != 0 || g[9] != 7 || g[17] != 1 {
		t.Errorf("the hands: % x", g[5:20])
	}
	if g[20] != 0xFF || g[26] != 0 || g[25] != 0xFF || g[31] != 0 || g[21] != 7 || g[27] != 7 {
		t.Errorf("the armour: % x", g[20:32])
	}
}
