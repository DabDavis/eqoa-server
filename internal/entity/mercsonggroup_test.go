package entity

import (
	"testing"
	"time"
)

// Petition 2805: an aggressive bard merc's group songs land on its owner's
// group mates in reach and their mercs (merccall.go, GM ticket 2025: its heals
// already did), as a player bard's do; a group mate out of the song's reach
// does not take it. The control: the same bard on assist (or passive) keeps
// its songs on its owner's own alone (the owner, 2026-11-13).
func TestAnAggressiveBardMercSingsToItsGroup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stance MercStance
		mates  bool
	}{
		{"aggressive", MercAggressive, true},
		{"assist", MercAssist, false},
		{"passive", MercPassive, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newMercRig(t)
			anthem := bookSpell(800, "Anthem of Drive", 2, 2*time.Second, 5*time.Second, 50, "Group, Increase STR by 45 ; Increase STA by 45, Radius 30, 30sec")
			ms := r.hire(r.kit(classBard, 0, 0, anthem), tc.stance)
			sessB, bea := r.player("Bea", 0, 0)
			g := r.group(bea)
			k := r.kit(2, 0, 0) // a paladin
			k.Name = "Theirs"
			if _, why := r.w.HireMerc(sessB, k, MercPassive, r.at(0)); why != "" {
				t.Fatalf("Bea's hire: %s", why)
			}
			theirs := r.w.merc1(bea.ServerID)
			if theirs == nil || !groupHas(g, theirs.npc.ServerID) || !groupHas(g, ms.npc.ServerID) {
				t.Fatalf("the group %v", g.Members)
			}
			r.run(0, 10*time.Second)
			if !buffOn(r.p.Effects, &anthem) || !buffOn(ms.npc.Effects, &anthem) {
				t.Fatalf("the anthem is not on its owner (%v) and the bard (%v)", buffOn(r.p.Effects, &anthem), buffOn(ms.npc.Effects, &anthem))
			}
			if got := buffOn(bea.Effects, &anthem); got != tc.mates {
				t.Errorf("Bea, a group mate: the anthem %v, want %v", got, tc.mates)
			}
			if got := buffOn(theirs.npc.Effects, &anthem); got != tc.mates {
				t.Errorf("Bea's merc: the anthem %v, want %v", got, tc.mates)
			}
			if !tc.mates {
				return
			}
			// Bea 400 off: the anthem sung again does not reach her
			bea.X += 400
			r.w.mu.Lock()
			bea.Effects = nil
			r.w.mu.Unlock()
			r.run(10*time.Second, 40*time.Second)
			if buffOn(bea.Effects, &anthem) {
				t.Error("Bea, 400 off, took the anthem")
			}
		})
	}
}

// An aggressive merc's other group buffs reach its group mates the same way;
// its buff at one target stays its owner's own.
func TestAnAggressiveMercsGroupBuffReachesItsGroup(t *testing.T) {
	r := newMercRig(t)
	gra := mercGroupBuffSpell()
	ms := r.hire(r.kit(9, 0, 0, gra), MercAggressive)
	_, bea := r.player("Bea", 0, 0)
	r.group(bea)
	r.run(0, 10*time.Second)
	if !buffOn(r.p.Effects, &gra) || !buffOn(bea.Effects, &gra) {
		t.Fatalf("the group buff: owner %v, Bea %v", buffOn(r.p.Effects, &gra), buffOn(bea.Effects, &gra))
	}
	single := bookSpell(700805, "Order", 1, 5*time.Second, 0, 60, "Target, Increase AC by 650, Increase Maximum HP by 650, 30min")
	r.w.mu.Lock()
	r.w.mercHelpLocked(ms, &single, bea.ServerID, r.at(11*time.Second))
	r.w.mu.Unlock()
	if buffOn(bea.Effects, &single) {
		t.Error("a merc's single-target buff landed on a group mate")
	}
}
