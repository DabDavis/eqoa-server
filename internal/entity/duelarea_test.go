package entity

import (
	"math"
	"testing"
	"time"
)

// zpFingersJSON is a Zaoran Plateau caster ability's workbook effects in the
// shape of its spelleffect row (Fingers of Thunder's): a target-based area,
// a hit and two second hits, each scaled by DEX and the caster's level.
const zpFingersJSON = `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"radius":10,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[` +
	`{"id":"03A6","type":"Dmg/Heal","kind":"hp","amount":-100,"scale":"dex","scale_pct":200,"level_pct":1317,"resist":"lr"},` +
	`{"id":"03A6","type":"Dmg/Heal","kind":"hp2","amount":-1,"scale":"dex","scale_pct":200,"level_pct":649,"resist":"lr"},` +
	`{"id":"03A7","type":"Dmg/Heal","kind":"hp2","amount":-1,"scale":"dex","scale_pct":200,"level_pct":649,"resist":"lr","scope":"target"}]}`

// zpFingers is that spell as a cast builds it (castSpellFrom: its area read
// from its workbook effects).
func zpFingers(t *testing.T) CastSpell {
	t.Helper()
	sp := CastSpell{SpellID: 900778, DisplayIndex: 6, Scope: 1, Name: "Fingers", CastTime: 2 * time.Second,
		Recast: 3 * time.Second, Range: 30}
	sp.Def = SpellDefFrom(sp.Name, "", "", zpFingersJSON)
	sp.Area = ReadSpellArea(sp.Name, "", sp.Def)
	if sp.Area.Centre != AreaOnTarget || !sp.Def.Harmful || len(sp.Def.Effects) != 3 {
		t.Fatalf("the test's fingers read as area %+v, effects %+v", sp.Area, sp.Def.Effects)
	}
	return sp
}

// zpDuelHit is the fingers' figure from c at a share: each hit's spell
// amount (its base, DEX and level shares) at the share, rounded half up.
func zpDuelHit(sp CastSpell, c *Player, share float64) int {
	n := 0
	for _, e := range sp.Def.Effects {
		n += int(math.Floor(float64(-spellAmount(e, c))*share + 0.5))
	}
	return n
}

// Petition 2647: an area spell aimed at a duel opponent (the Zaoran Plateau
// fingers, a target-based area) landed nothing, for areaStartLocked kept a
// player target only when a flagged foe, so the opponent was dropped from its
// own cast. Now it lands each hit at DuelDamageShare; a creature beside the
// opponent still takes the splash and a bystander nothing. The controls: a
// single-target strike lands as it did (33 halved, 17), and the same fingers
// at a player with no fight between them is refused.
func TestDuelAreaSpellAtTheOpponent(t *testing.T) {
	d := newDuelists(t)
	if d.w.StartCast(d.sb, zpFingers(t), d.bSeesA, d.at(0)) {
		t.Fatal("the fingers at a player were cast with no duel")
	}
	d.start(t)
	rat := &NPC{ServerID: 2002, Name: "rat", Level: 1, HP: 5000, MaxHP: 5000, X: 101, Y: 10, Z: 100}
	d.w.NPCs[rat.ServerID] = rat
	d.w.SpatialIndex.Insert(rat.ServerID, rat.X, rat.Z)
	d.c.X, d.c.Z = 101, 101 // a bystander in the area

	sp := zpFingers(t)
	want := zpDuelHit(sp, d.b, DuelDamageShare)
	if full := zpDuelHit(sp, d.b, 1); want <= 0 || want*2 < full-3 || want*2 > full+3 {
		t.Fatalf("the test's figures: %d in the duel against %d whole", want, full)
	}
	a0, c0 := d.a.HP, d.c.HP
	cast := d.at(2 * time.Second)
	if !d.w.StartCast(d.sb, sp, d.bSeesA, cast) {
		t.Fatalf("the fingers at the opponent were refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(cast.Add(sp.CastTime + castLand))
	d.w.mu.Unlock()
	if taken := a0 - d.a.HP; taken != want {
		t.Fatalf("the fingers took %d of the opponent, want %d (the whole hit at the duel's share)", taken, want)
	}
	if d.c.HP != c0 {
		t.Fatalf("a bystander took %d", c0-d.c.HP)
	}
	if rat.HP >= 5000 {
		t.Fatalf("the creature beside the opponent took no splash")
	}

	// the control: a single-target strike, unchanged
	before := d.a.HP
	st := strike()
	st.SpellID = 651
	at := d.at(10 * time.Second)
	if !d.w.StartCast(d.sb, st, d.bSeesA, at) {
		t.Fatalf("the strike was refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(at.Add(st.CastTime + castLand))
	d.w.mu.Unlock()
	if taken := before - d.a.HP; taken != 17 {
		t.Fatalf("the strike took %d, want 33 halved, 17", taken)
	}
}

// The same fingers between two flagged players land at PvPDamageShare (the
// PvP fight's path, which kept a flagged foe before the fix too).
func TestPvPAreaSpellAtTheFoe(t *testing.T) {
	d := newPvP(t)
	d.flag(t, d.sa, 0)
	d.flag(t, d.sb, 0)
	sp := zpFingers(t)
	want := zpDuelHit(sp, d.b, PvPDamageShare)
	a0 := d.a.HP
	cast := d.at(2 * time.Minute)
	if !d.w.StartCast(d.sb, sp, d.bSeesA, cast) {
		t.Fatalf("the fingers at the foe were refused: %q", duelLines(duelDrain(d.sb)))
	}
	d.w.mu.Lock()
	d.w.castSystem(cast.Add(sp.CastTime + castLand))
	d.w.mu.Unlock()
	if taken := a0 - d.a.HP; taken != want {
		t.Fatalf("the fingers took %d of the foe, want %d", taken, want)
	}
}
