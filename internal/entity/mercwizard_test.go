package entity

import "testing"

// A wizard merc's spells do 50% of their damage (the GM Jibwiz's petition
// 1907: "Nerf the damage per cast on the mercenary wizard by 35%", and 1954:
// "an additional 15% across the board"), its melee and shots 85%; a
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
	if want := mag * 50 / 100; wiz != want {
		t.Errorf("the wizard's bolt did %d, want %d (50%% of the magician's %d)", wiz, want, mag)
	}
	wiz13, mag10 := &mercState{kit: MercKit{Class: 13}}, &mercState{kit: MercKit{Class: 10}}
	if a, b := mercMeleeShare(wiz13, 1000), mercMeleeShare(mag10, 1000); a != 850 || b != 1000 {
		t.Errorf("melee 1000: the wizard's %d (want 850), the magician's %d (want 1000)", a, b)
	}
	if got := mercSpellShare(&mercState{kit: MercKit{Class: 13}}, 1); got != 1 {
		t.Errorf("a wizard's 1 is %d, want at least 1", got)
	}
}
