package entity

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// mercloadout.go: a merc's ten spell slots. Set, it casts only what they
// hold; none set, its whole book; the default ten keep the AI's picks.

// A wizard with a big nuke and a small one casts the big one (none set: the
// control); with only the small one on its slots it never casts the big one,
// and casts the small one.
func TestMercCastsOnlyFromItsTenSlots(t *testing.T) {
	for _, slotted := range []bool{false, true} {
		r := newMercRig(t)
		big := bookSpell(951, "Ice Comet", 1, 2*time.Second, 10*time.Second, 40, "Target, Decrease Hitpoints by 900")
		small := bookSpell(952, "Frost Bolt", 1, 2*time.Second, 10*time.Second, 40, "Target, Decrease Hitpoints by 100")
		ms := r.hire(r.kit(13, 0, 0, big, small), MercAssist)
		if slotted {
			if !r.w.SetMercLoadout(r.sess, ms.slot, []int{small.SpellID, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
				t.Fatal("no merc of the slot")
			}
		}
		crab := r.creature("Desert Crab", 28, 10)
		crab.HP, crab.MaxHP = 900000, 900000
		r.p.AttackTarget, r.p.AutoAttack = crab.ServerID, true
		r.run(0, 30*time.Second)
		_, castBig := ms.ready[big.SpellID]
		_, castSmall := ms.ready[small.SpellID]
		switch {
		case !slotted && !castBig:
			t.Error("none set: the big nuke was never cast (the control)")
		case slotted && castBig:
			t.Error("the big nuke, off its slots, was cast")
		case slotted && !castSmall:
			t.Error("the slotted nuke was never cast")
		}
	}
}

// The default ten are the AI's picks: the first spell of each use its book
// casts (its turn takes the first of its kind) is among them, so a merc with
// them set picks as it did with none set; past ten, the least worth goes.
func TestMercDefaultLoadoutKeepsTodaysPicks(t *testing.T) {
	r := newMercRig(t)
	var spells []CastSpell
	for i := range 12 {
		spells = append(spells, bookSpell(960+i, fmt.Sprintf("Bolt %d", i), 1, 2*time.Second, 10*time.Second, 40,
			fmt.Sprintf("Target, Decrease Hitpoints by %d", 100+50*i)))
	}
	heal := bookSpell(980, "Mend", 1, 2*time.Second, 10*time.Second, 40, "Target, Increase Hitpoints by 400")
	dot := bookSpell(981, "Burn", 1, 2*time.Second, 10*time.Second, 40, "Target, Decrease Hitpoints by 20 per tic, 30sec")
	spells = append(spells, heal, dot)
	k := r.kit(9, 0, 0, spells...)
	k.Offensive = true
	def := MercDefaultLoadout(k)
	if len(def) != MercLoadoutSlots {
		t.Fatalf("%d slots", len(def))
	}
	firsts := func(kit MercKit) map[mercUse]int {
		ms := &mercState{kit: kit, body: newMercBody(1, &kit)}
		out := map[mercUse]int{}
		for _, sp := range ms.mercBook() {
			if _, ok := out[sp.use]; !ok {
				out[sp.use] = sp.s.SpellID
			}
		}
		return out
	}
	today := firsts(k)
	if len(today) < 2 {
		t.Fatalf("the book's uses %v: the rig lost one", today)
	}
	for use, id := range today {
		if !slices.Contains(def, id) {
			t.Errorf("use %d's pick %d is not in the default %v", use, id, def)
		}
	}
	set := k
	set.Loadout = def
	if got := firsts(set); fmt.Sprint(got) != fmt.Sprint(today) {
		t.Errorf("with the default set the picks are %v, %v with none", got, today)
	}
	if slices.Contains(def, 960) {
		t.Errorf("the weakest bolt (960) is in the default %v over a stronger one", def)
	}
	// the control: a loadout of the weakest alone changes the pick
	weak := k
	weak.Loadout = []int{960, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := firsts(weak); got[useNuke] != 960 {
		t.Errorf("a loadout of bolt 960 alone picks %v", got)
	}
	// a spell turned off in its menu is never in the default
	off := k
	off.SpellsOff = []int{today[useNuke]}
	if slices.Contains(MercDefaultLoadout(off), today[useNuke]) {
		t.Error("a spell turned off is in the default")
	}
}

// The window's book lists its fight book, then its pets' and the rest, each
// once; its default is what the window shows while none is set.
func TestMercWindowBookAndLoadoutState(t *testing.T) {
	r := newMercRig(t)
	nuke := bookSpell(950, "Ice Bolt", 1, 2*time.Second, 10*time.Second, 40, "Target, Decrease Hitpoints by 300")
	ms := r.hire(r.kit(13, 0, 0, nuke), MercAssist)
	book, slots, set, ok := r.w.MercLoadout(r.sess, ms.slot)
	if !ok || set || len(book) != 1 || book[0].Spell.SpellID != 950 || book[0].Use != "hit" || slots[0] != 950 {
		t.Fatalf("book %+v slots %v set %v ok %v", book, slots, set, ok)
	}
	r.w.SetMercLoadout(r.sess, ms.slot, make([]int, MercLoadoutSlots))
	if _, slots, set, _ = r.w.MercLoadout(r.sess, ms.slot); !set || slots[0] != 0 {
		t.Errorf("cleared: slots %v set %v", slots, set)
	}
	if spells, _ := r.w.MercSpells(r.sess, ms.slot); len(spells) != 1 || !spells[0].Off {
		t.Errorf("the PS2's Spells box shows a spell off its slots as on: %+v", spells)
	}
}
