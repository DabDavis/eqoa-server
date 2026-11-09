package entity

import (
	"fmt"
	"testing"
	"time"
)

// npcshoot.go and the sic's casts (gmtriggers.go's SetNPCOn, classtemplate.go's
// castsAt), on the class templates' rig.

// archerBow is a longbow's model (0xDEBA8F46: Windwalker's Longbow and the rest).
const archerBow = -558198970

// archerRig is the template rig with a ranger's template ("archer", class 1).
func archerRig(t *testing.T) *templateRig {
	t.Helper()
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities, note, source) VALUES ('archer', 1, '[]', '', 'test')`); err != nil {
		t.Fatal(err)
	}
	return r
}

// holds gives an NPC its hands as LoadNPCs gives a row's (npcHandTypes: the
// catalog's ranged model a 7, anything else a 1).
func holds(n *NPC, primary, secondary int) {
	n.PrimaryWeapon, n.SecondaryWeapon, n.PrimaryType, n.SecondaryType = primary, secondary, 0, 0
	if primary != 0 {
		n.PrimaryType = 1
	}
	switch {
	case secondary == archerBow:
		n.SecondaryType = 7
	case secondary != 0:
		n.SecondaryType = 1
	}
}

// A creature of a ranger's template holding a bow in its second hand and nothing in
// its first shoots: GetSwing's ranged swing for its level (Range4 at 65, Range2 at
// 25), the Range effect's name hashed for its 0x00B1, an arrow a hit (Range4 two).
// The controls, all at 65 with the bow: the same template with a sword in its first
// hand too, the roamer's template (a warrior's), no template: each Attack4.
func TestARangersTemplateShootsTheBowInItsHand(t *testing.T) {
	r := archerRig(t)
	archer, armed, roamer, plain := r.npc("Gnoll Brute"), r.npc("Gnoll Guard"), r.npc("Gnoll Priest"), r.npc("Gnoll Pyro")
	r.assign("gnoll brute", "archer")
	r.assign("gnoll guard", "archer")
	r.assign("gnoll priest", "roamer")
	holds(archer, 0, archerBow)
	holds(armed, 0x0D54F88C, archerBow)
	holds(roamer, 0, archerBow)
	holds(plain, 0, archerBow)
	for _, n := range []*NPC{archer, armed, roamer, plain} {
		n.Level = 65
	}
	if !npcShoots(archer) || npcSwingKind(archer) != 11 {
		t.Fatalf("the archer: shoots %v, swing %d; want Range4 (11)", npcShoots(archer), npcSwingKind(archer))
	}
	length, delay, fx, hits := npcRound(archer)
	if length != 5*time.Second || delay != 5*time.Second || fx != HashResourceID("Range4") ||
		fmt.Sprint(hits) != fmt.Sprint([]time.Duration{500 * time.Millisecond, time.Second}) {
		t.Errorf("the archer's round: %v, %v, %#x, hits %v; want 5 s, Range4 %#x, two arrows", length, delay, fx, hits, HashResourceID("Range4"))
	}
	archer.Level = 25
	if k := npcSwingKind(archer); k != 9 || len(npcShotHits(k)) != 1 {
		t.Errorf("a level 25 archer: swing %d, %d hits; want Range2 (9), one arrow", k, len(npcShotHits(k)))
	}
	archer.Level = 65
	for _, c := range []struct {
		name string
		n    *NPC
	}{{"a sword in its first hand too", armed}, {"the roamer's template", roamer}, {"no template", plain}} {
		if npcShoots(c.n) || npcSwingKind(c.n) != 5 {
			t.Errorf("the control with %s: shoots %v, swing %d; want Attack4 (5)", c.name, npcShoots(c.n), npcSwingKind(c.n))
		}
		if _, _, fx, hits := npcRound(c.n); fx != HashResourceID("Attack4") || len(hits) != 4 {
			t.Errorf("the control with %s: %#x, %d hits; want Attack4's four", c.name, fx, len(hits))
		}
	}
}

// The archer fights from its bow's reach: a player 20 off is in it, so it stands
// where it is and shoots (its 0x00B1 the Range4 effect, two hits a round); one 60
// off it runs to, standing just inside 25. The control, a creature of the same
// level without the bow, runs in to its melee reach at a player 20 off.
func TestAnArcherStandsOffAtItsBowsReach(t *testing.T) {
	r := archerRig(t)
	archer, melee := r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
	r.assign("gnoll brute", "archer")
	holds(archer, 0, archerBow)
	archer.Level, melee.Level = 65, 65
	for _, n := range []*NPC{archer, melee} {
		n.HP, n.MaxHP = 100000, 100000
	}
	p, sess := r.player("Target", archer.X+20, archer.Z)
	q, _ := r.player("Other", melee.X+20, melee.Z)
	p.HP, p.MaxHP, q.HP, q.MaxHP = 1_000_000, 1_000_000, 1_000_000, 1_000_000
	r.fight(archer, p.ServerID)
	r.fight(melee, q.ServerID)
	x0 := archer.X
	var shots, hits int
	for d := time.Duration(0); d <= 12*time.Second; d += 100 * time.Millisecond {
		r.w.TickCount++
		r.tick(d)
		msgs := drain(sess)
		for _, c := range castsIn(msgs) {
			if c.caster == r.objectID(archer.ServerID) {
				if c.fx != HashResourceID("Range4") || c.ms != 5000 || c.target != sess.PlayerObjectID() {
					t.Errorf("the archer's swing at %v: %+v", d, c)
				}
				shots++
			}
		}
		for _, f := range damageIndications(msgs) {
			if f[0] == int64(r.objectID(archer.ServerID)) && f[2] == int64(sess.PlayerObjectID()) && f[1] < 0 {
				hits++
			}
		}
	}
	if archer.X != x0 || shots < 2 || hits != 2*shots {
		t.Errorf("the archer 20 off: moved %.2f, %d shots, %d hits; want it standing, shooting two arrows a round", archer.X-x0, shots, hits)
	}
	if d := distXZ(melee.X, melee.Z, q.X, q.Z); d > npcReach(melee) {
		t.Errorf("the control stands %.2f off its player; want within its melee reach %.2f", d, npcReach(melee))
	}

	// 60 off: it runs to its bow's reach, not to arm's length
	p.X = archer.X + 60
	r.w.SpatialIndex.Update(p.ServerID, p.X-60+20, p.Z, p.X, p.Z)
	for d := 13 * time.Second; d <= 25*time.Second; d += 100 * time.Millisecond {
		r.w.TickCount++
		r.tick(d)
	}
	if d := distXZ(archer.X, archer.Z, p.X, p.Z); d < npcShootReach-1 || d > npcShootReach {
		t.Errorf("the archer stands %.2f off a player it ran at; want just inside %.0f", d, npcShootReach)
	}
}

// A trigger's sic with casts has its spawn fight the NPC by its whole template: the
// necromancer taps it every 5 s from 1 s in (the 0x00B1 at the NPC, the drained
// share back on the caster), and the roamer crits it on its clock; an area spell
// takes the NPC alone, a player beside it untouched. The control, the same sic
// without casts: melee alone, no cast, no crit.
func TestASicWithCastsFightsByTheWholeTemplate(t *testing.T) {
	for _, casts := range []bool{true, false} {
		r := newTemplateRig(t)
		necro, pup := r.npc("Gnoll Necro"), r.npc("Gnoll Pup")
		brute, guard := r.npc("Gnoll Brute"), r.npc("Gnoll Guard")
		mage, priest := r.npc("Gnoll Mage"), r.npc("Gnoll Priest")
		guard.X, guard.SpawnX = brute.X+3, brute.X+3
		priest.X, priest.SpawnX = mage.X+3, mage.X+3
		r.w.SpatialIndex.Update(guard.ServerID, 3000, 100, guard.X, guard.Z)
		r.w.SpatialIndex.Update(priest.ServerID, 4000, 100, priest.X, priest.Z)
		r.assign("gnoll necro", "necromancer")
		r.assign("gnoll brute", "roamer")
		r.assign("gnoll mage", "wizard")
		for _, n := range []*NPC{pup, guard, priest, brute, mage} {
			n.HP, n.MaxHP = 100000, 100000
		}
		necro.HP, necro.MaxHP = 50000, 100000 // hurt, and enough to outlast the pup's melee
		_, sess := r.player("Watcher", 100, 110)
		by, bySess := r.player("Bystander", priest.X+0.5, priest.Z+1)
		by.HP, by.MaxHP = 100000, 100000
		for _, set := range []struct {
			n      *NPC
			target string
		}{{necro, "gnoll pup"}, {brute, "gnoll guard"}, {mage, "gnoll priest"}} {
			if _, ok := r.w.SetNPCOn(set.n.ServerID, set.target, 30, casts); !ok {
				t.Fatalf("casts %v: SetNPCOn %s on %s found none", casts, set.n.Name, set.target)
			}
		}
		var taps, crits, storms, stormOthers, heals, lost int
		last := necro.HP
		for d := time.Duration(0); d <= 40*time.Second; d += 100 * time.Millisecond {
			r.w.TickCount++
			r.tick(d)
			if necro.HP > last {
				heals++ // only a tap's drain heals it here (no regen runs)
			}
			last = necro.HP
			for _, f := range damageIndications(drain(bySess)) {
				if f[2] == int64(bySess.PlayerObjectID()) {
					lost++
				}
			}
			for _, c := range castsIn(drain(sess)) {
				switch {
				case c.caster == r.objectID(necro.ServerID) && c.fx == fxSiphon:
					if c.target != r.objectID(pup.ServerID) {
						t.Errorf("casts %v: the tap at %#x, not the pup", casts, c.target)
					}
					taps++
				case c.caster == r.objectID(brute.ServerID) && c.fx == templateSpecialFX && c.ms == 0:
					crits++
				case c.caster == r.objectID(mage.ServerID) && c.fx == fxFireStorm:
					storms++
					stormOthers += len(c.others)
				}
			}
		}
		if casts {
			if taps != 8 || heals != 8 || crits < 1 || storms < 2 || stormOthers != 0 || by.HP != by.MaxHP || lost != 0 {
				t.Errorf("with casts: %d taps, %d heals, %d crits, %d storms with %d others, the bystander at %d of %d; want 8 taps and heals, a crit, "+
					"storms on the priest alone", taps, heals, crits, storms, stormOthers, by.HP, by.MaxHP)
			}
		} else if taps != 0 || heals != 0 || crits != 0 || storms != 0 {
			t.Errorf("the control (no casts): %d taps, %d heals, %d crits, %d storms; want its melee alone", taps, heals, crits, storms)
		}
		if pup.CurrentEnemy != necro.ServerID || guard.CurrentEnemy != brute.ServerID || pup.HP >= pup.MaxHP {
			t.Errorf("casts %v: the pup fights %d (HP %d of %d), the guard %d; want each fighting back", casts, pup.CurrentEnemy, pup.HP, pup.MaxHP, guard.CurrentEnemy)
		}
	}
}
