package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A potion whose line names a place (potion.go: EQOAEmu's Surefall Potion,
// 2026-11-01) against the gate tests' world (newGateFixture: a wizard of 50
// at (1000, 50, 1000), a group of its own, the travel hook recording). The
// places, rows, items and people are made up.

func hollowPotion(line string) PotionEffect {
	return NewPotionEffect(950001, "Hollow Potion", line, "", 0, "", 0, 0)
}

// The line's place is a spellgates row's place: the drinker alone goes
// there, a self gate's caster, at the next tick's travels (the member beside
// it stays), and the potion is drunk, not thrown. The control: before the
// travels run nothing is handed over.
func TestAPotionTakesItsDrinkerToTheNamedPlace(t *testing.T) {
	f := newGateFixture(t)
	f.member(1002, "Friend", 1003, 50, 1000)
	f.gate(database.SpellGate{Spell: "Moonwell Circle", Kind: database.GateGroup, Place: "Glimmer Hollow", World: 0,
		X: 5000, Y: 40, Z: 6000})
	pe := hollowPotion("Self, Teleport to Glimmer Hollow")
	if pe.Thrown() || pe.Def.Harmful {
		t.Fatalf("read as thrown %v, harmful %v: want a drunk potion that harms no one", pe.Thrown(), pe.Def.Harmful)
	}
	refusal, err := f.w.UsePotion(f.sess, pe, 0, f.t0)
	if err != nil || refusal != "" {
		t.Fatalf("refused %q (%v)", refusal, err)
	}
	if len(f.sent) != 0 {
		t.Fatalf("the control: handed %v before the travels ran", f.travellers())
	}
	f.w.runTravels()
	if got := f.travellers(); len(got) != 1 || got[0] != "Wizard" {
		t.Fatalf("sent %v, want the drinker alone", got)
	}
	tr := f.sent[0]
	if tr.To.World != 0 || tr.To.X != 5000 || tr.To.Y != 40 || tr.To.Z != 6000 || tr.To.Place != "Glimmer Hollow" ||
		tr.Ring != 0 || tr.Home != 0 || tr.Spell != "Hollow Potion" || tr.Sess != f.sess {
		t.Errorf("travel %+v, want the row's place (no ground: as written), by the potion's name", tr)
	}

	// the row by its spell's name names the same place
	f.sent = nil
	if r, _ := f.w.UsePotion(f.sess, hollowPotion("Self, Teleport to the Moonwell Circle"), 0, f.t0); r != "" {
		t.Fatalf("by the row's spell: refused %q", r)
	}
	f.w.runTravels()
	if len(f.sent) != 1 || f.sent[0].To.Place != "Glimmer Hollow" {
		t.Errorf("by the row's spell: sent %+v", f.sent)
	}
}

// A place no row is refuses the potion before anything is drunk (it stays
// on its stack: the handler takes one only for ""), and so does a place in a
// world the drinker's game cannot load; two rows of one place, the first by
// its spell. The control: a line that names no place sends no one.
func TestAPotionToNoPlaceIsRefused(t *testing.T) {
	f := newGateFixture(t)
	if r, err := f.w.UsePotion(f.sess, hollowPotion("Self, Teleport to Nowhere Vale"), 0, f.t0); err != nil || r != potionNoPlace {
		t.Errorf("no row: refused %q (%v), want %q", r, err, potionNoPlace)
	}
	f.w.runTravels()
	if len(f.sent) != 0 || len(f.caster.Effects) != 0 {
		t.Errorf("no row: sent %v, %d effects", f.travellers(), len(f.caster.Effects))
	}

	f.w.SetLoadableWorlds(func(*session.Session) func(uint8) bool { return func(w uint8) bool { return w < 6 } })
	f.gate(database.SpellGate{Spell: "Far Shore Circle", Kind: database.GateGroup, Place: "a far shore", World: 7, X: 1, Y: 2, Z: 3})
	if r, _ := f.w.UsePotion(f.sess, hollowPotion("Self, Teleport to a far shore"), 0, f.t0); r != "Your game cannot load a far shore." {
		t.Errorf("world 7 for a game of six: refused %q", r)
	}

	f.gate(database.SpellGate{Spell: "Zephyr Circle", Kind: database.GateGroup, Place: "Twin Oaks", World: 0, X: 300, Y: 1, Z: 300})
	f.gate(database.SpellGate{Spell: "Aster Circle", Kind: database.GateGroup, Place: "Twin Oaks", World: 0, X: 100, Y: 1, Z: 100})
	if r, _ := f.w.UsePotion(f.sess, hollowPotion("Self, Teleport to Twin Oaks"), 0, f.t0); r != "" {
		t.Fatalf("two rows: refused %q", r)
	}
	f.w.runTravels()
	if len(f.sent) != 1 || f.sent[0].To.X != 100 {
		t.Errorf("two rows of one place: sent %+v, want the aster circle's (the first by its spell)", f.sent)
	}

	// the control: a potion of no place
	f.sent = nil
	if r, _ := f.w.UsePotion(f.sess, hollowPotion("Self, Increase Power by 10"), 0, f.t0); r != "" {
		t.Fatalf("the control: refused %q", r)
	}
	f.w.runTravels()
	if len(f.sent) != 0 {
		t.Errorf("the control: sent %v", f.travellers())
	}
}
