package entity

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Magi'Kot's summoning gem (716990, tools/evil_melee_20261118.py; the owner's
// Evil Melee epic, the forum's t155: "The Evil Melee doing the quest will hand
// out gems that will port you to PoS (the usual first island, where the Isle of
// Wonder port is)"): its itemeffect line is a self use, not thrown, harming no
// one, SPENT at each use (one gem a trip), to the Sky Portal spellgates row
// (the main isle), its user alone.
func TestMagiKotsSummoningGemTakesItsUserAloneToTheMainIsle(t *testing.T) {
	const line = "Self, Teleport to Sky Portal"
	pe := NewPotionEffect(716990, "Magi'Kot's summoning gem", line, "", 0, "", 0, 0)
	if pe.Thrown() || pe.Def.Harmful {
		t.Fatalf("read as thrown %v, harmful %v: want a self use", pe.Thrown(), pe.Def.Harmful)
	}
	if pe.Def.Unlimited {
		t.Errorf("read as unlimited: a gem is one trip")
	}
	place, ok := potionPlace(&pe.Def)
	if !ok || !strings.EqualFold(place, "Sky Portal") || database.SpellGateKey(place) != "sky portal" {
		t.Fatalf("place %q (%v), want the spellgates row 'sky portal'", place, ok)
	}
	f := newGateFixture(t)
	f.member(1002, "Friend", 1003, 50, 1000)
	f.gate(database.SpellGate{Spell: "sky portal", Kind: database.GateGroup, Place: "the Plane of Sky", World: 4,
		X: 4834.8, Y: 250.1, Z: 5524.7, Radius: 30})
	if r, err := f.w.UsePotion(f.sess, pe, 0, f.t0); err != nil || r != "" {
		t.Fatalf("refused %q (%v)", r, err)
	}
	f.w.runTravels()
	if got := f.travellers(); len(got) != 1 || got[0] != "Wizard" {
		t.Fatalf("sent %v, want the user alone", got)
	}
	if to := f.sent[0].To; to.World != 4 || to.X != 4834.8 || to.Z != 5524.7 {
		t.Errorf("sent to %+v, want the main isle", to)
	}
}
