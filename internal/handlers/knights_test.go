package handlers

import (
	"bytes"
	"slices"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A knight (the paladin and the shadow knight) holds a weapon in its primary
// hand alone (the user, 2026-10-30: "for player Shadowknights they should not
// be able to equip a weapon in their secondary hand. they can only wear it in
// their primary, but secondary can be used in either hand. Same for paladins
// ... they can wear shields in their secondary hand of course"): a weapon
// either hand may hold goes into the primary, taking the place of the sword
// there, and a shield goes on beside it. The control: a warrior's goes into
// its free secondary hand, beside the sword.
func TestAKnightHoldsAWeaponInItsPrimaryHandAlone(t *testing.T) {
	for _, c := range []struct {
		name  string
		class Class
		mace  int // the hand the mace goes in
		sword int // where the sword is after (-1 off)
	}{
		{"a shadow knight", ClassShadowKnight, 14, -1},
		{"a paladin", ClassPaladin, 14, -1},
		{"a warrior", ClassWarrior, 15, 14},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newGearFixture(t)
			f.player.Class = int(c.class)
			sword := f.own(f.pattern("Blade", 12, 1, 0), 14)
			mace := f.own(f.pattern("rusty mace", 14, 3, 0), -1)
			shield := f.own(f.pattern("Buckler", 13, 0, 0), -1)
			f.loadList()
			equip := func(key uint32) [][]byte {
				return f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
					w.WriteUint32(key)
					w.WriteUint32(0xFFFFFFFF)
					w.WriteByte(0)
				})
			}
			// list: the sword 0, the mace 1, the shield 2
			out := equip(1)
			if len(out) != 1 || !bytes.Equal(out[0], []byte{0x3F, 0x00, 1, byte(c.mace)}) {
				t.Fatalf("the mace: %x, want it into hand %d", out, c.mace)
			}
			w := f.worn()
			if w[c.mace] != mace || c.sword < 0 && slices.Contains([]int{w[14], w[15]}, sword) || c.sword >= 0 && w[c.sword] != sword {
				t.Fatalf("worn %v: want the mace in %d and the sword in %d", w, c.mace, c.sword)
			}
			if out := equip(2); len(out) != 1 || !bytes.Equal(out[0], []byte{0x3F, 0x00, 2, 17}) {
				t.Fatalf("the shield: %x, want it on (17)", out)
			}
			// beside a knight's mace; a warrior's off-hand mace it takes off
			// (Msg003F's table: a shield empties the secondary hand)
			w = f.worn()
			if w[17] != shield || c.mace == 14 && w[14] != mace || c.mace == 15 && w[15] != 0 {
				t.Errorf("worn %v after the shield: want it in 17, the mace in 14 kept and in 15 taken off", w)
			}
		})
	}
	// only the paladin and the shadow knight
	for class := range 15 {
		if got, want := offHandWeaponBarred(class), class == 2 || class == 3; got != want {
			t.Errorf("class %d barred %v, want %v", class, got, want)
		}
	}
	// bait and a held item are no weapons: a knight's go where they always did
	for _, c := range []struct{ wear, slot int }{{wearBait, 15}, {18, 19}} {
		items := []database.CharInventoryEntry{row(9, 9, c.wear, -1, 0, "x")}
		if plan, err := planInteractItem(items, newInvList(items), 0, int(ClassShadowKnight)); err != nil || plan.slot != c.slot {
			t.Errorf("a knight's wear %d: slot %d (%v), want %d", c.wear, plan.slot, err, c.slot)
		}
	}
}

// A weapon set made with a weapon in the off hand (before the rule) puts its
// main hand on and not that weapon, which the knight is told; the control: a
// warrior's takes both up (TestUseWeaponSetEquipsTheSet).
func TestAKnightsWeaponSetLeavesTheOffHandWeapon(t *testing.T) {
	f := newGearFixture(t)
	f.player.Class = int(ClassPaladin)
	swordP, daggerP := f.pattern("Blade", 14, 1, 0), f.pattern("Dirk", 14, 3, 0)
	sword, dagger := f.own(swordP, 14), f.own(daggerP, 15)
	f.loadList()
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(0)
		writeWideString(w, "Dual", 64)
	})
	for _, id := range []int{sword, dagger} {
		if err := f.inv.SetEquipLoc(f.sid, id, -1); err != nil {
			t.Fatal(err)
		}
	}
	out := f.send(protocol.OpcodeUseWeaponSet, func(w *buffer.Writer) { w.WriteByte(0) })
	if len(out) != 2 || !bytes.Equal(out[0], []byte{0x3F, 0x00, 0, 14}) || !bytes.Contains(out[1], utf16le(offHandRefusal)) {
		t.Fatalf("a paladin's set: %x, want the sword into 14 and %q", out, offHandRefusal)
	}
	if w := f.worn(); w[14] != sword || len(w) != 1 {
		t.Errorf("worn %v: want the sword alone", w)
	}
}

// #template gives a knight a shield (or a held item) in the other hand, never
// a second weapon; the control: a warrior's takes the second sword over the
// shield worth less.
func TestTheTemplateGivesAKnightNoSecondWeapon(t *testing.T) {
	sword := item(1, 14, 0, 10, 500)
	weakShield := item(6, 13, 10, 0, 0)
	for _, c := range []struct {
		class Class
		want  map[int][]int
	}{
		{ClassWarrior, map[int][]int{14: {1}, 15: {1}}},
		{ClassShadowKnight, map[int][]int{14: {1}, 17: {6}}},
		{ClassPaladin, map[int][]int{14: {1}, 17: {6}}},
	} {
		got := wornBySlot(templateGear([]database.ItemPatternRecord{sword, weakShield}, int(c.class)))
		ok := len(got) == len(c.want)
		for s, ids := range c.want {
			ok = ok && slices.Equal(got[s], ids)
		}
		if !ok {
			t.Errorf("class %d: %v, want %v", c.class, got, c.want)
		}
	}
	if got := wornBySlot(templateGear([]database.ItemPatternRecord{sword}, int(ClassShadowKnight))); len(got) != 1 || !slices.Equal(got[14], []int{1}) {
		t.Errorf("a shadow knight with a sword alone: %v, want it in the primary hand alone", got)
	}
}
