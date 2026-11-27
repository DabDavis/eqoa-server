package entity

import "testing"

// A tank merc's base stats go to STA and AGI, a melee fighter's to STR, DEX
// and AGI (mercroles.go), each above what a player of its class spends there
// (SimBaseStats) and within the level's cap; a caster's are a player's (the
// control). A tank's resists come to the cap, what its WIS adds counted.
func TestMercRoleStats(t *testing.T) {
	for _, level := range []int{10, 30, 60} {
		war, sim := MercBaseStats(0, 0, level), SimBaseStats(0, 0, level)
		if war[1] < sim[1] || war[2] <= sim[2] || war[0] > sim[0] {
			t.Errorf("a warrior of %d: %v, a player's %v: want more STA and AGI, no more STR", level, war, sim)
		}
		rog, rsim := MercBaseStats(6, 0, level), SimBaseStats(6, 0, level)
		if rog[0] < rsim[0] || rog[2] <= rsim[2] || rog[3] < rsim[3] || rog[1] > rsim[1] {
			t.Errorf("a rogue of %d: %v, a player's %v: want more STR, AGI and DEX", level, rog, rsim)
		}
		if wiz := MercBaseStats(13, 0, level); wiz != SimBaseStats(13, 0, level) {
			t.Errorf("a wizard of %d: %v, not a player's", level, wiz)
		}
		for i, v := range war {
			if v > StatCapBase(level) {
				t.Errorf("a warrior of %d: stat %d at %d over the cap", level, i, v)
			}
		}
	}
	g := Gear{FR: 400}
	MercTankResists(&g, 70) // 10 from its WIS
	if g.PR != ResistCap-10 || g.FR != 400 || g.AR != ResistCap-10 {
		t.Errorf("resists %+v", g)
	}
	if MercTankAC(60) != 2400 || MercTankAC(10) != 400 {
		t.Errorf("tank AC %d, %d", MercTankAC(60), MercTankAC(10))
	}
}

func TestMercDPSReductionByRole(t *testing.T) {
	for _, tc := range []struct {
		name    string
		class   int
		reduced bool
	}{
		{"ranger", 1, true}, {"monk", 4, true}, {"bard", 5, true}, {"rogue", 6, true},
		{"magician", 10, true}, {"necromancer", 11, true}, {"enchanter", 12, true},
		{"wizard", 13, true}, {"alchemist", 14, true},
		{"warrior", 0, false}, {"paladin", 2, false}, {"shadow knight", 3, false},
		{"druid", 7, false}, {"shaman", 8, false}, {"cleric", 9, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newMercRig(t)
			fixDice(t)
			spell := bookSpell(950, "Test strike", 1, 0, 0, 1, "Target, Decrease Hitpoints by 300")
			ms := r.hire(r.kit(tc.class, 0, 0, spell), MercAssist)
			target := r.creature("a crab", 28, 3)
			target.MaxHP, target.HP = 100_000, 100_000
			hits := func() [2]int {
				r.w.mu.Lock()
				defer r.w.mu.Unlock()
				start := target.HP
				r.w.mercMeleeHitLocked(ms, target, r.t0)
				afterMelee := target.HP
				r.w.mercHarmLocked(ms, &spell, target, r.t0)
				return [2]int{start - afterMelee, afterMelee - target.HP}
			}
			normal := hits()
			r.w.SetMercDPSReductionPct(25)
			reduced := hits()
			for i, n := range normal {
				if n <= 1 {
					t.Fatalf("hit %d did only %d damage before tuning", i, n)
				}
				want := n
				if tc.reduced {
					want = (3*n + 2) / 4 // 75%, rounded to the nearest hit point
				}
				if reduced[i] != want {
					t.Errorf("hit %d did %d damage, want %d (previously %d)", i, reduced[i], want, n)
				}
			}
		})
	}
}

func TestMercDPSReductionAfterFormula(t *testing.T) {
	r := newMercRig(t)
	ms := mercWithPet(t, r, MercAssist)
	target := r.creature("a crab", 30, 3)
	target.MaxHP, target.HP = 10_000, 10_000
	own := &petState{owner: r.p, kind: petKindOurs, npc: &NPC{ServerID: 4950, Owner: r.p.ServerID, HP: 100, MaxHP: 100}}
	writeFormulas(t, r.w, "formulas:\n  merc_dot: base + 20\n  pet_proc: base + 20\n")
	r.w.SetDamageFormulaMode(FormulaLive)
	r.w.SetMercDPSReductionPct(25)

	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.putNPCLocked(own.npc)
	hp := target.HP
	r.w.mercDamageLocked(ms, target, 100, numberDoT, r.t0)
	if got := hp - target.HP; got != 90 {
		t.Errorf("mercenary tick did %d damage, want 90", got)
	}
	hp = target.HP
	r.w.petLandHitLocked(ms.pet, target, 100, r.t0, false, numberProc)
	if got := hp - target.HP; got != 90 {
		t.Errorf("mercenary pet proc did %d damage, want 90", got)
	}
	hp = target.HP
	r.w.petLandHitLocked(own, target, 100, r.t0, false, numberProc)
	if got := hp - target.HP; got != 120 {
		t.Errorf("player pet proc did %d damage, want 120", got)
	}
	if zero, one := r.w.mercRoleDamage(ms, 0), r.w.mercRoleDamage(ms, 1); zero != 0 || one != 1 {
		t.Errorf("0 and 1 damage resolved to %d and %d", zero, one)
	}
}
