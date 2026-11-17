package handlers

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// Weapon Swap Back (gearswap.go, work board 1581). Each test checks that every
// charinventory row is the same after the presses as before them, but for
// equiploc: nothing is made, split, merged or lost.

// press takes set up, with vanilla's swap-back byte when back.
func (f *gearFixture) press(set byte, back bool) [][]byte {
	return f.send(protocol.OpcodeUseWeaponSet, func(w *buffer.Writer) {
		w.WriteByte(set)
		if back {
			w.WriteByte(gearSwapBack)
		}
	})
}

// makeSet makes set n of the hands as worn now.
func (f *gearFixture) makeSet(n byte, name string) {
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(n)
		writeWideString(w, name, 64)
	})
}

// wear puts the rows in their slots (-1 off) in the database alone.
func (f *gearFixture) wear(rows ...[2]int) {
	f.t.Helper()
	for _, r := range rows {
		if err := f.inv.SetEquipLoc(f.sid, r[0], r[1]); err != nil {
			f.t.Fatal(err)
		}
	}
}

// rows is every charinventory row but its equiploc.
func (f *gearFixture) rows() string {
	f.t.Helper()
	q, err := f.db.Query(`SELECT itemid, serverid, patternid, stackleft, location FROM charinventory ORDER BY itemid`)
	if err != nil {
		f.t.Fatal(err)
	}
	defer q.Close()
	var b bytes.Buffer
	for q.Next() {
		var id, sid, pat, stack, loc int
		if err := q.Scan(&id, &sid, &pat, &stack, &loc); err != nil {
			f.t.Fatal(err)
		}
		fmt.Fprintf(&b, "%d:%d:%d:%d:%d ", id, sid, pat, stack, loc)
	}
	return b.String()
}

func msgs(m ...[]byte) [][]byte { return m }

func sameMsgs(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// The user's case: two swords of the same pattern, a in 14 and b in 15; set 0
// the bow. With the bit, the set's second press puts a and b back as they were
// (never swapped), and the button toggles.
func TestWeaponSwapBackPutsTheSameSwordsBack(t *testing.T) {
	f := newGearFixture(t)
	swordP, bowP := f.pattern("Blade", 14, 1, 0), f.pattern("Longbow", 16, 7, 0)
	a, b := f.own(swordP, 14), f.own(swordP, 15)
	bow := f.own(bowP, -1)
	f.loadList() // a 0, b 1, bow 2
	f.wear([2]int{a, -1}, [2]int{b, -1}, [2]int{bow, 18})
	f.makeSet(0, "Bow")
	f.wear([2]int{bow, -1}, [2]int{a, 14}, [2]int{b, 15})
	start := f.rows()

	if out := f.press(0, true); !sameMsgs(out, msgs([]byte{0x3F, 0x00, 2, 18})) {
		t.Fatalf("the bow's set: %x", out)
	}
	if out := f.press(0, false); len(out) != 0 {
		t.Fatalf("the set again without the bit: %x, want nothing", out)
	}
	for round := 0; round < 3; round++ {
		out := f.press(0, true)
		if !sameMsgs(out, msgs([]byte{0x3F, 0x00, 0, 14}, []byte{0x3F, 0x00, 1, 15})) {
			t.Fatalf("round %d, back: %x", round, out)
		}
		if w := f.worn(); w[14] != a || w[15] != b || w[18] != 0 || len(w) != 2 {
			t.Fatalf("round %d, back: worn %v, want a in 14, b in 15", round, w)
		}
		if out := f.press(0, true); !sameMsgs(out, msgs([]byte{0x3F, 0x00, 2, 18})) {
			t.Fatalf("round %d, the bow again: %x", round, out)
		}
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}

// Bare hands is a set like the others: it sheathes, and pressed again draws.
func TestWeaponSwapBackBareHandsSheathesAndDraws(t *testing.T) {
	f := newGearFixture(t)
	sword := f.own(f.pattern("Blade", 12, 1, 0), 14)
	shield := f.own(f.pattern("Buckler", 13, 0, 0), 17)
	f.loadList()
	start := f.rows()
	if out := f.press(4, true); !sameMsgs(out, msgs([]byte{0x40, 0x00, 14}, []byte{0x40, 0x00, 17})) {
		t.Fatalf("sheathe: %x", out)
	}
	f.press(4, true)
	if w := f.worn(); w[14] != sword || w[17] != shield || len(w) != 2 {
		t.Fatalf("drawn: %v", w)
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}

// A row gone since is skipped, and another row of the same pattern is never
// worn in its place; with none left nothing moves but a line.
func TestWeaponSwapBackSkipsRowsThatAreGone(t *testing.T) {
	f := newGearFixture(t)
	swordP, bowP := f.pattern("Blade", 14, 1, 0), f.pattern("Longbow", 16, 7, 0)
	a, b := f.own(swordP, 14), f.own(swordP, 15)
	bow := f.own(bowP, -1)
	spare := f.own(swordP, -1)
	f.loadList()
	f.wear([2]int{a, -1}, [2]int{b, -1}, [2]int{bow, 18})
	f.makeSet(0, "Bow")
	f.wear([2]int{bow, -1}, [2]int{a, 14}, [2]int{b, 15})
	f.press(0, true)
	if err := f.inv.DeleteCharItem(f.sid, b); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	f.press(0, true)
	if w := f.worn(); w[14] != a || w[15] != 0 || len(w) != 1 {
		t.Fatalf("b gone: worn %v, want a alone (never the spare %d)", w, spare)
	}

	// the bow over a, then a banked: the press sends only the line, the bow stays
	f.press(0, true)
	if _, err := f.db.Exec(`UPDATE charinventory SET location = ?, equiploc = -1 WHERE itemid = ?`, database.LocBank, a); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	start := f.rows()
	out := f.press(0, true)
	for _, m := range out {
		if op := opOf(m); op == 0x003F || op == 0x0040 {
			t.Fatalf("a banked: %x, want no equip or unequip", out)
		}
	}
	if len(out) == 0 {
		t.Fatal("a banked: no line")
	}
	if w := f.worn(); w[18] != bow || len(w) != 1 {
		t.Fatalf("a banked: worn %v, want the bow", w)
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}

// Hands changed in between: the press takes the set up again. A set that
// changes nothing leaves the memory; bare hands pressed after becomes it.
func TestWeaponSwapBackFollowsTheHands(t *testing.T) {
	f := newGearFixture(t)
	swordP, bowP := f.pattern("Blade", 14, 1, 0), f.pattern("Longbow", 16, 7, 0)
	a := f.own(swordP, 14)
	bow := f.own(bowP, -1)
	f.loadList()
	f.wear([2]int{a, -1}, [2]int{bow, 18})
	f.makeSet(0, "Bow")
	f.makeSet(1, "Bow too")
	f.wear([2]int{bow, -1}, [2]int{a, 14})
	start := f.rows()

	f.press(0, true) // the bow; memory: set 0, a -> bow
	f.wear([2]int{bow, -1}, [2]int{a, 14})
	f.press(0, true) // by hand a again: the bow taken up, not back
	if w := f.worn(); w[18] != bow || len(w) != 1 {
		t.Fatalf("hands changed: worn %v, want the bow", w)
	}
	f.press(0, true)
	if w := f.worn(); w[14] != a || len(w) != 1 {
		t.Fatalf("then back: worn %v, want a", w)
	}
	f.press(0, true)                            // the bow again
	if out := f.press(1, true); len(out) != 0 { // the same bow: nothing
		t.Fatalf("set 1, the same bow: %x", out)
	}
	f.press(0, true) // set 0's memory kept: a back
	if w := f.worn(); w[14] != a || len(w) != 1 {
		t.Fatalf("set 0 after set 1: worn %v, want a", w)
	}
	f.press(0, true) // the bow
	f.press(4, true) // bare hands: now the memory
	f.press(0, true) // set 0 is no longer the memory: the bow, not a
	if w := f.worn(); w[18] != bow || len(w) != 1 {
		t.Fatalf("set 0 after bare hands: worn %v, want the bow", w)
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}

// Order: a wand in 20 takes 18 off; knives in 18 take nothing off. Back puts
// the wand in first.
func TestWeaponSwapBackOrdersTheRows(t *testing.T) {
	f := newGearFixture(t)
	knives := f.own(f.pattern("Knives", 17, 10, 0), 18)
	wand := f.own(f.pattern("Wand", 18, 0, 0), 20)
	axe := f.own(f.pattern("Greataxe", 15, 2, 0), -1)
	f.loadList() // knives 0, wand 1, axe 2
	f.wear([2]int{knives, -1}, [2]int{wand, -1}, [2]int{axe, 16})
	f.makeSet(0, "Big")
	f.wear([2]int{axe, -1}, [2]int{knives, 18}, [2]int{wand, 20})
	start := f.rows()
	f.press(0, true)
	if w := f.worn(); w[16] != axe || len(w) != 1 {
		t.Fatalf("the axe: worn %v", w)
	}
	out := f.press(0, true)
	if !sameMsgs(out, msgs([]byte{0x3F, 0x00, 1, 20}, []byte{0x3F, 0x00, 0, 18})) {
		t.Fatalf("back: %x, want the wand in 20 first, then the knives in 18", out)
	}
	if w := f.worn(); w[18] != knives || w[20] != wand || len(w) != 2 {
		t.Fatalf("back: worn %v", w)
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}

func TestEquipOrder(t *testing.T) {
	row := func(id, wear, loc int) *database.CharInventoryEntry {
		return &database.CharInventoryEntry{ItemID: id, EquipLoc: loc, Pattern: database.ItemPatternRecord{EquipSlot: wear}}
	}
	inv := []*database.CharInventoryEntry{row(1, 17, -1), row(2, 18, -1), row(3, 16, -1), row(4, 12, -1)}
	got, ok := equipOrder(inv, []handItem{{1, 18}, {2, 20}})
	if !ok || len(got) != 2 || got[0].itemID != 2 || got[1].itemID != 1 {
		t.Fatalf("knives + wand: %v %v, want the wand first", got, ok)
	}
	if got, ok := equipOrder(inv, []handItem{{3, 18}, {4, 14}}); ok {
		t.Fatalf("bow + primary sword: %v, want no order", got)
	}
	if got, ok := equipOrder(inv, nil); !ok || len(got) != 0 {
		t.Fatalf("nothing: %v %v", got, ok)
	}
}

// A knight's either-hand weapon never goes back in the secondary hand.
func TestWeaponSwapBackKeepsTheKnightsRule(t *testing.T) {
	f := newGearFixture(t)
	f.player.Class = int(ClassPaladin)
	swordP, bowP := f.pattern("Blade", 14, 1, 0), f.pattern("Longbow", 16, 7, 0)
	a, b := f.own(swordP, 14), f.own(swordP, 15)
	bow := f.own(bowP, -1)
	f.loadList()
	f.wear([2]int{a, -1}, [2]int{b, -1}, [2]int{bow, 18})
	f.makeSet(0, "Bow")
	f.wear([2]int{bow, -1}, [2]int{a, 14}, [2]int{b, 15})
	start := f.rows()
	f.press(0, true)
	f.press(0, true)
	if w := f.worn(); w[14] != a || len(w) != 1 {
		t.Fatalf("a paladin back: worn %v, want a in 14 alone", w)
	}
	if got := f.rows(); got != start {
		t.Fatalf("rows changed:\n%s\nwant\n%s", got, start)
	}
}
