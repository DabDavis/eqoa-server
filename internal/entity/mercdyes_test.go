package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 2998: a merc wears its owner's dyes (mercdyes.go): the armour's, a trim's and the robe's, on
// the live look's colours and, for a PS2, the nearest disc tint (channel bytes 99-104); a change of the
// owner's dyes is on it at the next channel with no rekit; a hidden part (/merc look off) has none.
// The controls: the hands' dyes stay the owner's alone, and an owner with no dyes leaves its own look.
func TestMercWearsItsOwnersDyes(t *testing.T) {
	r := newMercRig(t)
	m := r.hire(r.kit(0, 0, 0), MercAssist).npc
	const red, blue, gold = 0xC01010FF, 0x1020C0FF, 0xD0A020FF
	r.w.mu.Lock()
	if d := r.w.npcDyesLocked(m); d != nil {
		t.Fatalf("an owner with no dyes: %v", *d)
	}
	r.w.mu.Unlock()

	var d Dyes
	d[database.DyeChest], d[database.DyeHelm], d[database.DyeChestTrim], d[database.DyePrimary] = red, blue, gold, red
	if _, ok := r.w.SetDyes(r.p.CharServerID, d); !ok {
		t.Fatal("owner not in the world")
	}
	r.w.mu.Lock()
	got := *r.w.npcDyesLocked(m)
	buf, _ := r.w.serializeEntityDiscLocked(r.sess, m.ServerID, 0x00B00001)
	r.w.mu.Unlock()
	if got[database.DyeChest] != red || got[database.DyeHelm] != blue || got[database.DyeChestTrim] != gold || got[database.DyePrimary] != 0 {
		t.Fatalf("the merc's dyes: %v", got)
	}
	if buf[99] != nearestTint(red) || buf[104] != nearestTint(blue) {
		t.Errorf("a PS2's tints: chest %d helm %d, want %d %d", buf[99], buf[104], nearestTint(red), nearestTint(blue))
	}

	// the owner dyes its chest blue: the merc's next channel has it
	d[database.DyeChest] = blue
	r.w.SetDyes(r.p.CharServerID, d)
	r.w.mu.Lock()
	if c := r.w.npcDyesLocked(m)[database.DyeChest]; c != blue {
		t.Errorf("after the owner's change: chest %08X", c)
	}
	// its look hidden: no dye on what it hides
	m.HiddenParts = MercArmorLook
	if dd := r.w.npcDyesLocked(m); dd != nil {
		t.Errorf("look hidden, dyes %v", *dd)
	}
	m.HiddenParts = HideHelm
	if dd := r.w.npcDyesLocked(m); dd == nil || dd[database.DyeHelm] != 0 || dd[database.DyeChest] != blue {
		t.Errorf("helm hidden, dyes %v", dd)
	}
	r.w.mu.Unlock()
}
