package handlers

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// Petition 1376 (Zaeire's, a rogue of 60 holding a dagger in each hand):
// "While dual wielding, if you swap say and equip a different secondary
// weapon, its replacing the primary if its listed as a secondary weapon.
// Weapon in the right hand is being swapped (primary) rather than the default
// left hand (secondary)". A weapon the item window calls SECONDARY (wear 14,
// word 340: either hand may hold it) went into the primary hand whenever both
// hands were full, the first slot of its pair; it goes into the secondary hand
// now, as CLIENT's EquipSlotFor has it, and the primary hand's weapon stays.
// The rest is as it was: a SECONDARY weapon goes into a free hand, the primary
// first; a PRIMARY one (wear 12) takes the primary hand's place and leaves the
// secondary; a 2HAND (wear 15) takes both off, and a SECONDARY one after it
// takes its place in the primary hand; one beside a shield takes the shield's
// place; a knight's goes into its primary hand alone; bait goes into the
// secondary. Each through the handler, as the client asks (0x003E by the
// record's key), against a database; the item names are made up.
func TestASecondaryWeaponTakesTheSecondaryHandsPlace(t *testing.T) {
	type item struct {
		name       string
		wear, slot int
	}
	right := item{"right dagger", 14, 14}
	left := item{"left dagger", 14, 15}
	for _, c := range []struct {
		name  string
		class Class
		worn  []item
		equip item           // put on from the pack (its slot unused)
		slot  int            // the hand 0x003F names
		after map[int]string // every hand worn after, by item name
	}{
		// the petition: both hands full, a SECONDARY weapon
		{"a rogue's third secondary weapon", ClassRogue, []item{right, left}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		{"beside a primary weapon", ClassRogue, []item{{"sword", 12, 14}, left}, item{"new dagger", 14, -1},
			15, map[int]string{14: "sword", 15: "new dagger"}},
		{"a warrior's", ClassWarrior, []item{right, left}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		{"a ranger's", ClassRanger, []item{right, left}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		{"a monk's", ClassMonk, []item{right, left}, item{"new fist", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new fist"}},
		{"a bard's", ClassBard, []item{right, left}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		{"a cleric's (any class but a knight holds two here)", ClassCleric, []item{right, left}, item{"new mace", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new mace"}},
		// a SECONDARY weapon into a free hand, as before
		{"the primary hand alone full", ClassRogue, []item{right}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		{"the secondary hand alone full", ClassRogue, []item{left}, item{"new dagger", 14, -1},
			14, map[int]string{14: "new dagger", 15: "left dagger"}},
		{"empty hands", ClassRogue, nil, item{"new dagger", 14, -1},
			14, map[int]string{14: "new dagger"}},
		// a PRIMARY weapon, a 2HAND, a shield's place, as before
		{"a primary weapon with both hands full", ClassRogue, []item{right, left}, item{"new sword", 12, -1},
			14, map[int]string{14: "new sword", 15: "left dagger"}},
		{"a two-hander with both hands full", ClassRogue, []item{right, left}, item{"greataxe", 15, -1},
			16, map[int]string{16: "greataxe"}},
		{"a secondary weapon after a two-hander", ClassRogue, []item{{"greataxe", 15, 16}}, item{"new dagger", 14, -1},
			14, map[int]string{14: "new dagger"}},
		{"beside a shield", ClassRogue, []item{right, {"buckler", 13, 17}}, item{"new dagger", 14, -1},
			15, map[int]string{14: "right dagger", 15: "new dagger"}},
		// a knight's weapon goes into its primary hand alone (offHandWeaponBarred)
		{"a shadow knight's beside a shield", ClassShadowKnight, []item{{"sword", 12, 14}, {"buckler", 13, 17}}, item{"new mace", 14, -1},
			14, map[int]string{14: "new mace", 17: "buckler"}},
		{"a paladin's beside a shield", ClassPaladin, []item{{"sword", 12, 14}, {"buckler", 13, 17}}, item{"new mace", 14, -1},
			14, map[int]string{14: "new mace", 17: "buckler"}},
		// bait is the secondary hand's alone
		{"bait with both hands full", ClassRogue, []item{right, left}, item{"bait", wearBait, -1},
			15, map[int]string{14: "right dagger", 15: "bait"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newGearFixture(t)
			f.player.Class, f.player.Level = int(c.class), 60
			names := map[int]string{}
			for _, w := range c.worn {
				names[f.own(f.pattern(w.name, w.wear, 5, 0), w.slot)] = w.name
			}
			names[f.own(f.pattern(c.equip.name, c.equip.wear, 5, 0), -1)] = c.equip.name
			f.loadList()
			key := uint32(len(c.worn)) // the list SendMemoryDump writes: creation order
			out := f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
				w.WriteUint32(key)
				w.WriteUint32(0xFFFFFFFF)
				w.WriteByte(0)
			})
			if len(out) != 1 || !bytes.Equal(out[0], []byte{0x3F, 0x00, byte(key), byte(c.slot)}) {
				t.Fatalf("0x003F %x, want %q into hand %d", out, c.equip.name, c.slot)
			}
			if !fitsEquip(c.equip.wear, c.slot) {
				t.Fatalf("hand %d fails CLIENT's fit check for wear %d", c.slot, c.equip.wear)
			}
			after := map[int]string{}
			for slot, id := range f.worn() {
				after[slot] = names[id]
			}
			if !reflect.DeepEqual(after, c.after) {
				t.Errorf("worn after %v, want %v", after, c.after)
			}
		})
	}
}

// clientEitherHandSlot is CLIENT's EquipSlotFor (0x00BDCEA0) for a wear-14
// item not yet worn: the second hand when the first holds anything
// (FindEquipped(list, 14, 0)) for a WAR, RAN, MNK, BRD or RGE of 20 and up,
// else the first. Its item window's stat preview (StatWithItem) equips the
// item there on a copy of the list, as Msg003F would.
func clientEitherHandSlot(worn map[int]bool, class Class, level int) int {
	switch class {
	case ClassWarrior, ClassRogue, ClassMonk, ClassRanger, ClassBard:
		if worn[equipPrimary] && level >= 20 {
			return equipSecondary
		}
	}
	return equipPrimary
}

// The hand a SECONDARY weapon goes into is CLIENT's own choice (EquipSlotFor)
// for every class CLIENT lets hold two, over every set of hands that may be
// full at once: the hands (14, 15), the two-hander (16), the shield (17), the
// ranged weapon (18) and the held items (19, 20). EquipSlotFor's gate (those
// five classes, from 20) is not this server's: every other class but the
// knights takes the same hand here, and a knight's goes into the primary hand
// alone. The first free hand of the pair, else the first (the rule until
// petition 1376), fails it wherever both hands are full.
func TestASecondaryWeaponGoesWhereClientsEquipSlotForPutsIt(t *testing.T) {
	if clientEitherHandSlot(map[int]bool{14: true, 15: true}, ClassRogue, 60) != equipSecondary {
		t.Fatal("the oracle: a rogue of 60's full hands take the second")
	}
	// one item for each hand, of a wear that goes there
	fill := []struct{ wear, slot int }{{12, 14}, {14, 15}, {15, 16}, {13, 17}, {16, 18}, {18, 19}, {18, 20}}
	for mask := 0; mask < 1<<len(fill); mask++ {
		var items []database.CharInventoryEntry
		worn := map[int]bool{}
		for i, h := range fill {
			if mask&(1<<i) != 0 {
				items = append(items, row(i+1, i+1, h.wear, h.slot, 0, fmt.Sprintf("worn %d", h.slot)))
				worn[h.slot] = true
			}
		}
		items = append(items, row(99, 99, 14, -1, 0, "new dagger"))
		for class := range Class(15) {
			plan, err := planInteractItem(items, newInvList(items), uint32(len(items)-1), int(class))
			if err != nil {
				t.Fatalf("hands %v, class %d: %v", worn, class, err)
			}
			var want int
			switch class {
			case ClassPaladin, ClassShadowKnight:
				want = equipPrimary // offHandWeaponBarred
			case ClassWarrior, ClassRogue, ClassMonk, ClassRanger, ClassBard:
				want = clientEitherHandSlot(worn, class, 60) // CLIENT's own
			default:
				want = clientEitherHandSlot(worn, ClassWarrior, 60) // ours: as any class's that may hold two
			}
			if !plan.equip || plan.slot != want {
				t.Errorf("hands %v, class %d: into %d, want %d", worn, class, plan.slot, want)
				continue
			}
			// and what comes off is Msg003F's: what is in the hand, then its table's
			var changes []equipChange
			for _, s := range append([]int{plan.slot}, equipDisplaces(14, plan.slot)...) {
				for _, e := range items {
					if e.EquipLoc == s {
						changes = append(changes, equipChange{e.ItemID, -1})
					}
				}
			}
			changes = append(changes, equipChange{99, plan.slot})
			if !reflect.DeepEqual(plan.changes, changes) {
				t.Errorf("hands %v, class %d: changes %v, want %v", worn, class, plan.changes, changes)
			}
		}
	}
}
