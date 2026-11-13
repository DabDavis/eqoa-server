package entity

import "testing"

// A wizard merc's spells do 65% of their damage (the GM Jibwiz's petition
// 1907: "Nerf the damage per cast on the mercenary wizard by 35%"); a
// magician merc's, the control, whole.
func TestMercWizardSpellDamageNerfed(t *testing.T) {
	lost := func(class int) int {
		r := newMercRig(t)
		nuke := bookSpell(950, "Ice Bolt", 1, 0, 0, 1, "Target, Decrease Hitpoints by 300")
		ms := r.hire(r.kit(class, 0, 0, nuke), MercPassive)
		crab := r.creature("Desert Crab", 28, 10)
		crab.HP, crab.MaxHP = 900000, 900000
		r.w.mu.Lock()
		r.w.mercHarmLocked(ms, &nuke, crab, r.at(0))
		r.w.mu.Unlock()
		return 900000 - crab.HP
	}
	mag, wiz := lost(10), lost(13)
	if mag <= 0 {
		t.Fatalf("the magician's bolt did %d", mag)
	}
	if want := mag * 65 / 100; wiz != want {
		t.Errorf("the wizard's bolt did %d, want %d (65%% of the magician's %d)", wiz, want, mag)
	}
	if got := mercSpellShare(&mercState{kit: MercKit{Class: 13}}, 1); got != 1 {
		t.Errorf("a wizard's 1 is %d, want at least 1", got)
	}
}
