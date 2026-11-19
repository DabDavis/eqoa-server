package entity

import "testing"

// An AoE taunt (area.go's isAreaTaunt; the owner, 2026-11-17: "aoe taunt for
// wars should area of effect taunt all mobs within 10m"). Goading Shout as the
// workbook gives it and as Heirloom's spellarea row 701079 had it (centre 1,
// round its target, radius 10: the EQOAEmu import's "AoERadius 10 (Target)"),
// cast at a creature 9 east of the warrior: every creature within 10 of the
// warrior (twelve, more than AreaMaxTargets' eight) is taunted, each with the
// warrior on top of a hate another player held; a creature beside the target
// but 13 off the warrior and one 11 off the warrior are not touched. The
// control: the same row on a storm (no taunt) still lies round its target.
func TestAnAoETauntTakesEveryCreatureWithinTenOfTheWarrior(t *testing.T) {
	noResist(t)
	goading := func(row *AreaRule) CastSpell {
		s := wbSpell(701079, 55, 1, "Goading Shout", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":55,"radius":10,
			"effects":[{"id":"081C","type":"Modify Hate","kind":"hate","amount":191,"scale":"str","scale_pct":16,"scope":"player_aoe"},
			{"id":"081D","type":"Modify Hate","kind":"hate","amount":954,"scale":"str","scale_pct":16}]}`)
		s.CastTime, s.Range = 0, 30
		if row != nil {
			s.Area = s.Area.With(*row)
		}
		return s
	}
	heirloomRow := &AreaRule{Centre: AreaRuleTarget, Radius: 10}
	for _, c := range []struct {
		name string
		row  *AreaRule
	}{{"the workbook's", nil}, {"Heirloom's row (round its target)", heirloomRow}} {
		r := newAreaRig(t)
		r.p.Class = 0 // a warrior
		me := r.p.ServerID
		r.player(1002, "Tmac", 1001, 1001)
		target := r.creature(2001, "fungal cavalier", 1009, 1000) // 9 east of the warrior
		var ring []*NPC
		for k := 0; k < 11; k++ { // eleven more round the warrior's west side, 3 to 9.5 off
			x, z := float32(1000-3-k/2), float32(1000-5+k)
			ring = append(ring, r.creature(EntityID(2100+k), "fungal hunter", x, z))
		}
		byTarget := r.creature(2201, "fungal hunter", 1013, 1000) // 4 beside the target, 13 off the warrior
		outside := r.creature(2202, "fungal hunter", 989, 1000)   // 11 west of the warrior
		all := append([]*NPC{target, byTarget, outside}, ring...)
		r.w.mu.Lock()
		for _, n := range all {
			n.HP, n.MaxHP = 500_000, 500_000
			r.w.updateAggro(n, 1002, 40_000) // a player on top of each
		}
		r.w.mu.Unlock()
		r.cast(t, goading(c.row), target.ServerID)
		for i, n := range append([]*NPC{target}, ring...) {
			if d := dist3(n.X, n.Y, n.Z, r.p.X, r.p.Y, r.p.Z); d > 10 {
				t.Fatalf("%s: the layout's creature %d stands %.1f off the warrior", c.name, i, d)
			}
			if n.CurrentEnemy != me || n.Threats[me] <= n.Threats[1002] {
				t.Errorf("%s: creature %d within 10 of the warrior: enemy %d, the warrior's threat %d against Tmac's %d",
					c.name, i, n.CurrentEnemy, n.Threats[me], n.Threats[1002])
			}
		}
		for _, n := range []*NPC{byTarget, outside} {
			if n.Threats[me] != 0 || n.CurrentEnemy != 1002 {
				t.Errorf("%s: %d (%.0f, %.0f), over 10 off the warrior, holds the warrior's threat %d (enemy %d)",
					c.name, n.ServerID, n.X, n.Z, n.Threats[me], n.CurrentEnemy)
			}
		}
	}

	// the control: a storm under the same row lies round its target
	r := newAreaRig(t)
	target := r.creature(2001, "fungal cavalier", 1009, 1000)
	byCaster := r.creature(2002, "fungal hunter", 997, 1000) // 3 off the caster, 12 off the target
	byTarget := r.creature(2003, "fungal hunter", 1013, 1000)
	storm := fireStorm()
	storm.Area = storm.Area.With(*heirloomRow)
	r.cast(t, storm, target.ServerID)
	if got := hits(byCaster, byTarget); !equalInts(got, []int{0, 36}) {
		t.Errorf("the storm under a target row: lost %v beside the caster and beside the target, want [0 36]", got)
	}
}

// isAreaTaunt: a harmful area of positive hate alone; not a storm, a hit with
// hate, a cone, a spell of one target, nor an area whose others take damage.
func TestWhatIsAnAreaTaunt(t *testing.T) {
	if s := tauntingShout(); !isAreaTaunt(&s) {
		t.Error("Taunting Shout is no area taunt")
	}
	for _, s := range []CastSpell{
		fireStorm(),
		spellOf(1, 1, "Sure Taunt", "Target, Increase Hate by 300"),
		spellOf(2, 1, "Hero's Shout", "Target, Decrease Hitpoints by 100, Increase Hate by 300, AOE"),
	} {
		if isAreaTaunt(&s) {
			t.Errorf("%q (%v) is an area taunt", s.Name, s.Area.Centre)
		}
	}
	cone := tauntingShout()
	cone.Area = cone.Area.With(AreaRule{Centre: AreaRuleCone})
	if isAreaTaunt(&cone) {
		t.Error("a cone is an area taunt")
	}
}
