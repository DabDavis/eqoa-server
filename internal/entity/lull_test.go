package entity

import (
	"testing"
	"time"
)

// Lulls (lull.go; the owner's ruling of 2026-10-29: "reduced the agro radius
// of a monster within or below the spells level range"): the words, a calmed
// creature's radius in its band and not above it, and a landing that is no
// attack. The spells, creatures and figures here are made up.

// lullSpell is a made-up lull of the level, cast at the caster's target.
func lullSpell(id, level int, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: level, Scope: 1, Name: "Made-up Hush", Icon: 0x3B3C3D3E,
		CastTime: 2 * time.Second, Recast: time.Second, PowerCost: 10, Range: 30, Def: ParseSpellNamed("Made-up Hush", line, "Reduces Aggression towards you.")}
}

// Every way the lines say a lull is read as one, cast at an enemy, with its
// time (a line with none lullDuration's) and its level; the area's figure is
// left as it was. The controls are no lull.
func TestTheLullWordsAreRead(t *testing.T) {
	for _, c := range []struct {
		line   string
		target SpellTarget
		dur    time.Duration
		level  int
		unread []string
	}{
		{"Target, Decreases Aggro Range, 20AE", TargetSingle, lullDuration, 0, []string{"20ae"}},
		{"Target, Decrease NPC Aggro Range, 14sec", TargetSingle, 14 * time.Second, 0, nil},
		{"Target, Decrease Aggro Range, 40sec", TargetSingle, 40 * time.Second, 0, nil},
		{"Lowers the awareness range of the target", TargetSingle, lullDuration, 0, nil},
		{"Target, Lull ; Increase 15% based on CHA, 16sec", TargetSingle, 16 * time.Second, 0, nil},
		{"Target, Lull up to level 33, 16sec", TargetSingle, 16 * time.Second, 33, nil},
		{"Target, Decrease NPC Aggro Range, up to level 47, 22sec", TargetSingle, 22 * time.Second, 47, nil},
	} {
		d := ParseSpellNamed("Made-up Hush", c.line, "Reduces Aggression towards you.")
		if len(d.Effects) != 1 || d.Effects[0].Kind != EffectLull || d.Effects[0].Amount != c.level || !d.Harmful ||
			d.Target != c.target || d.Duration != c.dur || len(d.Unread) != len(c.unread) {
			t.Errorf("%q: %+v", c.line, d)
			continue
		}
		for i := range c.unread {
			if d.Unread[i] != c.unread[i] {
				t.Errorf("%q leaves %q unread, want %q", c.line, d.Unread, c.unread)
			}
		}
		if a := ReadSpellArea("Made-up Hush", c.line, d); a.Centre != AreaNone {
			t.Errorf("%q is an area spell: %+v", c.line, a)
		}
	}
	if d := ParseSpellNamed("Made-up Hush", "Target, Lull ; Increase 15% based on CHA, 16sec", ""); d.Effects[0].Scale != StatCha || d.Effects[0].ScalePct != 15 {
		t.Errorf("the scale after a lull is not its own: %+v", d.Effects)
	}
	for _, line := range []string{"Target, Increase Aggro Range, 14sec", "Self, Increase AC by 10, up to level 30, 1min"} {
		d := ParseSpellNamed("Made-up Hush", line, "")
		for _, e := range d.Effects {
			if e.Kind == EffectLull {
				t.Errorf("%q read as a lull: %+v", line, d)
			}
		}
		if len(d.Unread) != 1 {
			t.Errorf("%q: unread %q", line, d.Unread)
		}
	}
}

// gnasher is the caster's fixture with its creature a level-n creature that
// attacks on sight, 10 off the druid (level 23), both in the spatial index.
func gnasher(t *testing.T, level int) *caster {
	c := newCaster(t)
	c.w.playerModels = map[uint32]bool{humanBody: true}
	n := c.npc
	n.Name, n.Level, n.ModelID, n.Faction, n.AggroRadius, n.LeashRadius = "Made-up Gnasher", level, int64(bearBody), FactionNeutral, 30, DefaultLeash
	n.HP, n.MaxHP = 900, 900
	n.X, n.SpawnX, n.SpawnY, n.SpawnZ = 110, 110, 10, 100
	c.w.SpatialIndex.Insert(c.p.ServerID, c.p.X, c.p.Z)
	c.w.SpatialIndex.Insert(n.ServerID, n.X, n.Z)
	return c
}

// look is one of the aggro scan's looks at the time given.
func (c *caster) look(at time.Duration) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.TickCount = aggroScanTicks * (c.w.TickCount/aggroScanTicks + 1)
	c.w.aggroSystem(c.at(at))
}

// stepTo moves the druid to x, the index with it.
func (c *caster) stepTo(x float32) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.SpatialIndex.Update(c.p.ServerID, c.p.X, c.p.Z, x, c.p.Z)
	c.p.X = x
}

// A level-10 lull (its band 30) on a level-20 creature: it lands with no hate
// and no fight, and the creature lets the druid stand 10 off, then attacks
// at 4 off; the control is the same creature unlulled, which attacks at 10.
func TestALullShrinksTheRadiusInItsBand(t *testing.T) {
	ctl := gnasher(t, 20)
	ctl.look(time.Second)
	if ctl.npc.CurrentEnemy != ctl.p.ServerID {
		t.Fatalf("the control: the creature 10 off did not attack (enemy %d)", ctl.npc.CurrentEnemy)
	}

	c := gnasher(t, 20)
	c.land(t, lullSpell(9701, 10, "Target, Decrease NPC Aggro Range, 30sec"), c.npcO, 0)
	if len(c.npc.Effects) != 1 || !c.npc.Effects[0].Lull {
		t.Fatalf("on the creature: %+v", c.npc.Effects)
	}
	if c.npc.CurrentEnemy != 0 || len(c.npc.Threats) != 0 || !c.p.LastCombatTime.IsZero() {
		t.Errorf("the lull was an attack: enemy %d, threats %v, the caster's last fight %v", c.npc.CurrentEnemy, c.npc.Threats, c.p.LastCombatTime)
	}
	c.look(5 * time.Second)
	if c.npc.CurrentEnemy != 0 {
		t.Errorf("the calmed creature attacked from 10 off")
	}
	c.stepTo(106)
	c.look(6 * time.Second)
	if c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("the calmed creature let the druid stand 4 off (enemy %d)", c.npc.CurrentEnemy)
	}

	// once it is over, its own radius again
	e := gnasher(t, 20)
	e.land(t, lullSpell(9702, 10, "Target, Decrease NPC Aggro Range, 30sec"), e.npcO, 0)
	e.look(40 * time.Second)
	if e.npc.CurrentEnemy != e.p.ServerID {
		t.Errorf("after the lull the creature 10 off did not attack (enemy %d)", e.npc.CurrentEnemy)
	}
}

// Above its band a lull calms nothing: a level-1 lull (band 21) on a level-25
// creature leaves it its radius, and adds no hate either; "up to level N"
// moves the band, and a creature already fighting is not calmed.
func TestALullAboveItsBandDoesNothing(t *testing.T) {
	c := gnasher(t, 25)
	c.land(t, lullSpell(9703, 1, "Target, Decreases Aggro Range, 20AE"), c.npcO, 0)
	if len(c.npc.Effects) != 0 || c.npc.CurrentEnemy != 0 || len(c.npc.Threats) != 0 {
		t.Fatalf("a lull above its band: effects %+v, enemy %d, threats %v", c.npc.Effects, c.npc.CurrentEnemy, c.npc.Threats)
	}
	c.look(5 * time.Second)
	if c.npc.CurrentEnemy != c.p.ServerID {
		t.Errorf("the creature above the band did not attack from 10 off")
	}

	u := gnasher(t, 25)
	u.land(t, lullSpell(9704, 1, "Target, Lull up to level 30, 16sec"), u.npcO, 0)
	if len(u.npc.Effects) != 1 || !u.npc.Effects[0].Lull {
		t.Errorf("a lull up to level 30 on a 25: %+v", u.npc.Effects)
	}

	f := gnasher(t, 20)
	f.npc.CurrentEnemy, f.npc.Threats = f.p.ServerID, map[EntityID]int64{f.p.ServerID: 5}
	f.land(t, lullSpell(9705, 10, "Target, Decrease NPC Aggro Range, 30sec"), f.npcO, 0)
	if len(f.npc.Effects) != 0 || f.npc.Threats[f.p.ServerID] != 5 {
		t.Errorf("a fighting creature: effects %+v, threats %v", f.npc.Effects, f.npc.Threats)
	}
}

// A lull calls no helper: the creature's own kind beside it, which a root's
// landing calls into the fight (the control), stays where it is.
func TestALullCallsNoHelper(t *testing.T) {
	for _, c := range []struct {
		line   string
		helped bool
	}{
		{"Target, Decrease NPC Aggro Range, 30sec", false},
		{"Target, Root, 30sec", true},
	} {
		f := gnasher(t, 20)
		f.w.SetSocialRules(map[string]SocialRule{"Made-up Gnasher": {Social: true, Faction: "Made-up Pack"}})
		helper := &NPC{ServerID: 2003, Name: "Made-up Gnasher", Level: 20, ModelID: int64(bearBody), HP: 900, MaxHP: 900,
			X: 112, Y: 10, Z: 100, SpawnX: 112, SpawnY: 10, SpawnZ: 100, Faction: FactionNeutral, AggroRadius: 30, LeashRadius: DefaultLeash}
		f.w.NPCs[helper.ServerID] = helper
		f.w.SpatialIndex.Insert(helper.ServerID, helper.X, helper.Z)
		f.land(t, lullSpell(9706, 10, c.line), f.npcO, 0)
		if got := helper.CurrentEnemy == f.p.ServerID; got != c.helped {
			t.Errorf("%q: the helper came %v, want %v (enemy %d)", c.line, got, c.helped, helper.CurrentEnemy)
		}
	}
}
