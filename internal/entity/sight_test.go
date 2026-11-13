package entity

import (
	"slices"
	"testing"
	"time"
)

// sightGround is a ground of boxes: nothing to stand on (a creature keeps its
// height), and each box stops a line that meets it.
type sightGround []sightBox

type sightBox struct{ x0, y0, z0, x1, y1, z1 float32 }

func (sightGround) Floor(_ uint8, _, y, _ float32) (float32, bool) { return y, false }

func (g sightGround) Sees(_ uint8, x0, y0, z0, x1, y1, z1 float32) bool {
	for _, b := range g {
		if b.meets([3]float32{x0, y0, z0}, [3]float32{x1, y1, z1}) {
			return false
		}
	}
	return true
}

func (b sightBox) meets(p, q [3]float32) bool {
	lo, hi := [3]float32{b.x0, b.y0, b.z0}, [3]float32{b.x1, b.y1, b.z1}
	t0, t1 := float32(0), float32(1)
	for i := range p {
		d := q[i] - p[i]
		if d == 0 {
			if p[i] < lo[i] || p[i] > hi[i] {
				return false
			}
			continue
		}
		a, c := (lo[i]-p[i])/d, (hi[i]-p[i])/d
		if a > c {
			a, c = c, a
		}
		t0, t1 = max(t0, a), min(t1, c)
		if t0 > t1 {
			return false
		}
	}
	return true
}

// A wall across x at w, floor to sky.
func wallAt(w float32) sightBox { return sightBox{w, -90000, -90000, w + 0.5, 90000, 90000} }

// A creature attacks on sight only what it can see (sight.go): not through a
// wall, nor up through a floor, nor down through one; over a low wall, under a
// beam at head height and round a wall's end it can. The control for each: the
// same two with nothing between them are in a fight.
func TestACreatureNoticesOnlyWhatItCanSee(t *testing.T) {
	for _, c := range []struct {
		what    string
		ground  sightGround
		py      float32
		attacks bool
	}{
		{"nothing between them", sightGround{}, 10, true},
		{"a wall between them", sightGround{wallAt(1005)}, 10, false},
		{"a wall over a head high", sightGround{{1005, 0, 900, 1005.5, 12, 1100}}, 10, false},
		{"a low wall, the head's line over it", sightGround{{1005, 0, 900, 1005.5, 11, 1100}}, 10, true},
		{"a beam at head height, the waist's line under it", sightGround{{1005, 11.3, 900, 1005.5, 12, 1100}}, 10, true},
		{"a wall that ends short of the line", sightGround{{1005, 0, 1001, 1005.5, 900, 1100}}, 10, true},
		{"a ceiling over the creature, the player upstairs", sightGround{{900, 14, 900, 1100, 15, 1100}}, 20, false},
		{"upstairs with the floor open", sightGround{{900, 14, 900, 1002, 15, 1100}}, 20, true},
		{"the creature upstairs, the player beneath it", sightGround{{900, 4, 900, 1100, 5, 1100}}, 0, false},
	} {
		f := newAggroFix(t, 10)
		f.p.Y = c.py
		bear := f.npc(2001, "Brown Bear", 10, bearBody, 1010, 1000)
		f.w.SetGround(c.ground)
		f.look()
		if f.attacks(bear) != c.attacks {
			t.Errorf("%s: attacks %v, want %v", c.what, f.attacks(bear), c.attacks)
		}
	}
	// a player who walks round the wall is seen at the next look
	f := newAggroFix(t, 10)
	bear := f.npc(2001, "Brown Bear", 10, bearBody, 1010, 1000)
	f.w.SetGround(sightGround{wallAt(1005)})
	f.look()
	if f.attacks(bear) {
		t.Fatal("seen through the wall")
	}
	f.p.X = 1007
	f.look()
	if !f.attacks(bear) {
		t.Error("not seen on its own side of the wall")
	}
	// no ground that knows sight blocks nothing, as before
	f = newAggroFix(t, 10)
	bear = f.npc(2001, "Brown Bear", 10, bearBody, 1010, 1000)
	f.w.SetGround(floorOnly{})
	f.look()
	if !f.attacks(bear) {
		t.Error("a ground that knows no sight stopped the look")
	}
}

// A heal behind a wall draws no one; in the open it draws the green creature it
// always did.
func TestAHealBehindAWallDrawsNoOne(t *testing.T) {
	for _, walled := range []bool{true, false} {
		f := newAggroFix(t, 30)
		f.p.HP = 500
		green := f.npc(2001, "Brown Bear", 20, bearBody, 1010, 1000)
		if walled {
			f.w.SetGround(sightGround{wallAt(1005)})
		}
		f.selfHeal(100)
		if f.attacks(green) == walled {
			t.Errorf("walled %v: drawn %v", walled, f.attacks(green))
		}
	}
}

// A social NPC comes when it sees the one attacked or its attacker, and stays
// behind a wall that hides both.
func TestAHelperMustSeeTheFight(t *testing.T) {
	f := newSocialFix(t)
	victim := f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	hidden := f.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	beside := f.npc(102, "Guard Cadwell", 30, humanBody, 1000, 1020)
	f.w.SetGround(sightGround{wallAt(1010)})
	f.hit(victim, 10)
	if f.attacks(hidden) {
		t.Error("the guard behind the wall came")
	}
	if !f.attacks(beside) {
		t.Error("the guard on the fight's side of the wall stayed")
	}

	// the attacker on the helper's side, the one attacked on the other
	f = newSocialFix(t)
	victim = f.npc(100, "Guard Alinon", 30, humanBody, 1000, 1000)
	sees := f.npc(101, "Guard Brenna", 30, humanBody, 1020, 1000)
	f.p.X = 1025
	f.w.SetGround(sightGround{wallAt(1010)})
	f.hit(victim, 10)
	if !f.attacks(sees) {
		t.Error("the guard who saw the attacker stayed")
	}
}

// A rock, a tree's trunk or a wall stops a player's area spell: round its
// target it takes what the target's side has and spares what stands behind, and
// its 0x00B1 names only those it reaches; the target itself, the centre, is hit
// though the wall stands between it and its caster.
func TestAWallStopsAPlayersArea(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
	beside := r.creature(2002, "Deathfist Pawn", 1014, 1000)
	behind := r.creature(2003, "Deathfist Pawn", 1027, 1000)
	r.w.SetGround(sightGround{wallAt(1024)})
	b1 := r.cast(t, fireStorm(), target.ServerID)
	if got, want := hits(target, beside, behind), []int{82, 36, 0}; !equalInts(got, want) {
		t.Errorf("lost %v, want %v (the target, one beside it, one behind the wall)", got, want)
	}
	if b1[10] != 1 {
		t.Errorf("the 0x00B1 names %d others, want the one", b1[10])
	}

	// its own target out of its caster's sight (a rock between them, or a
	// wall): the cast is refused before it begins (the owner, 2026-11-13: "no
	// line of sight, no cast"), nothing spent and no one reached
	for _, g := range []sightGround{{{1010, -90000, 998, 1011, 90000, 1002}}, {wallAt(1005)}} {
		r = newAreaRig(t)
		target = r.creature(2001, "Deathfist Pawn", 1020, 1000)
		r.w.SetGround(g)
		r.p.Casting, r.p.Lockouts = nil, nil
		if r.w.StartCast(r.sess, fireStorm(), r.objectID(target.ServerID), r.t0) {
			t.Errorf("an area round a target out of its caster's sight began (%v)", g)
		}
	}

	// a low rock, under the head's line and over the waist's, shelters no one
	// from an area, as it hides no one from a creature
	// (TestACreatureNoticesOnlyWhatItCanSee): petition 1882, a Blade Dance on
	// the frost giants' camp, each giant behind a stone 1.4 tall spared
	r = newAreaRig(t)
	target = r.creature(2001, "Deathfist Pawn", 1020, 1000)
	low := r.creature(2002, "Deathfist Pawn", 1027, 1000)
	r.w.SetGround(sightGround{{1024, 0, 900, 1024.5, 11, 1100}})
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(low); got[0] != 36 {
		t.Errorf("behind a low rock it lost %d, want 36", got[0])
	}
	// its own target behind the low rock from its caster is hit too
	r = newAreaRig(t)
	target = r.creature(2001, "Deathfist Pawn", 1020, 1000)
	r.w.SetGround(sightGround{{1005, 0, 900, 1005.5, 11, 1100}})
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(target); got[0] != 82 {
		t.Errorf("its target behind a low rock from its caster lost %d, want 82", got[0])
	}
	// and a rock as high as a head still spares
	r = newAreaRig(t)
	target = r.creature(2001, "Deathfist Pawn", 1020, 1000)
	tall := r.creature(2002, "Deathfist Pawn", 1027, 1000)
	r.w.SetGround(sightGround{{1024, 0, 900, 1024.5, 12, 1100}})
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(tall); got[0] != 0 {
		t.Errorf("behind a rock as high as a head it lost %d", got[0])
	}
}

// An area round its caster spares what is behind a wall from the caster, its
// own target too.
func TestAWallStopsACastersArea(t *testing.T) {
	spell := fireStorm()
	spell.Area.Centre = AreaOnCaster
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1010, 1000)
	near := r.creature(2002, "Deathfist Pawn", 1000, 1006)
	r.w.SetGround(sightGround{wallAt(1005)})
	r.cast(t, spell, target.ServerID)
	if got, want := hits(target, near), []int{0, 36}; !equalInts(got, want) {
		t.Errorf("lost %v, want %v (its target behind the wall, one on its side)", got, want)
	}
	// the control: with no wall its target takes its figure
	r = newAreaRig(t)
	target = r.creature(2001, "Deathfist Pawn", 1010, 1000)
	r.cast(t, spell, target.ServerID)
	if got := hits(target); got[0] != 82 {
		t.Errorf("with no wall its target lost %d", got[0])
	}
}

// Who gets behind something while the area is cast is spared at its landing.
func TestAnAreaLooksAgainAsItLands(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
	moved := r.creature(2002, "Deathfist Pawn", 1022, 1000)
	there := r.creature(2003, "Deathfist Pawn", 1014, 1000)
	r.w.SetGround(sightGround{wallAt(1024)})
	spell := fireStorm()
	if !r.w.StartCast(r.sess, spell, r.objectID(target.ServerID), r.t0) {
		t.Fatal("not begun")
	}
	moved.X = 1028
	r.w.mu.Lock()
	r.w.castSystem(r.t0.Add(spell.CastTime + castLand))
	r.w.mu.Unlock()
	if got, want := hits(moved, there), []int{0, 36}; !equalInts(got, want) {
		t.Errorf("lost %v, want %v (behind the wall since, one still in it)", got, want)
	}
}

// A group's spell is no area here: it finds its members behind a wall. The
// control: a helping area's others stop at it.
func TestAGroupSpellFindsItsOwnBehindAWall(t *testing.T) {
	land := func(scope int) int {
		r := newAreaRig(t)
		friend := r.player(1002, "Ally", 1010, 1000)
		r.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{r.p.ServerID, friend.ServerID}}}
		r.p.GroupID, friend.GroupID = 7, 7
		r.w.SetGround(sightGround{wallAt(1005)})
		cs := &castState{spell: CastSpell{Scope: scope, Name: "Test Heal", Area: SpellArea{Centre: AreaOnCaster, Radius: 30},
			Def: SpellDef{Effects: []SpellEffect{{Kind: EffectHP, Amount: 100}}}}, caster: r.p.ServerID, others: []EntityID{friend.ServerID}}
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOthersLocked(cs, r.p, r.sess, r.t0)
		return friend.HP - 500
	}
	if got := land(2); got <= 0 {
		t.Errorf("a group's spell healed its member behind a wall by %d", got)
	}
	if got := land(0); got != 0 {
		t.Errorf("an area that helps healed one behind a wall by %d", got)
	}
}

// A creature's area stops at a wall: round its target it spares who stands
// behind one from the target, named or not, the target itself hit; one who gets
// behind it during the cast is spared at the landing.
func TestAWallStopsACreaturesArea(t *testing.T) {
	r := newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage := r.npc("Gnoll Mage")
	target, ts := r.player("Target", 1001.5, 100)
	behind, bs := r.player("Behind", 1001.5, 108)
	r.w.SetGround(sightGround{{900, -100, 104, 1100, 900, 104.5}})
	r.fight(mage, target.ServerID)
	var told, spared []string
	for d := time.Duration(0); d <= 6*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
		told = append(told, linesOf(drain(ts))...)
		spared = append(spared, linesOf(drain(bs))...)
	}
	if len(told) == 0 {
		t.Error("the target took nothing")
	}
	if len(spared) != 0 || behind.HP != behind.MaxHP {
		t.Errorf("the one behind the wall: %q, HP %d", spared, behind.HP)
	}

	// the wall between them only once the cast has begun
	r = newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage = r.npc("Gnoll Mage")
	target, _ = r.player("Target", 1001.5, 100)
	behind, bs = r.player("Behind", 1001.5, 108)
	r.fight(mage, target.ServerID)
	for d := time.Duration(0); d <= 2*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	r.w.SetGround(sightGround{{900, -100, 104, 1100, 900, 104.5}})
	drain(bs)
	for d := 2100 * time.Millisecond; d <= 6*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if got := linesOf(drain(bs)); len(got) != 0 || behind.HP != behind.MaxHP {
		t.Errorf("behind the wall in time: %q, HP %d", got, behind.HP)
	}

	// hidden from the boss, in plain sight of whoever it is hitting: the user's
	r = newTemplateRig(t)
	r.assign("gnoll mage", "wizard")
	mage = r.npc("Gnoll Mage")
	target, _ = r.player("Target", 1001.5, 100)
	behind, bs = r.player("Behind", 1000, 106)
	r.w.SetGround(sightGround{{999, -100, 102, 1000.8, 900, 103}})
	r.fight(mage, target.ServerID)
	drain(bs)
	for d := time.Duration(0); d <= 6*time.Second; d += 100 * time.Millisecond {
		r.tick(d)
	}
	if got := linesOf(drain(bs)); len(got) != 0 || behind.HP != behind.MaxHP {
		t.Errorf("hidden from the boss and in sight of its target: %q, HP %d", got, behind.HP)
	}

	// its own target behind a wall from it, for either kind of area; a spell of
	// no area asks nothing, and with nothing between them nothing is spared
	r = newTemplateRig(t)
	mage = r.npc("Gnoll Mage")
	target, _ = r.player("Target", 1001.5, 100)
	r.w.SetGround(sightGround{wallAt(1000.5)})
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	isSpared := func(centre AreaCentre) bool {
		sp := &CastSpell{Area: SpellArea{Centre: centre, Radius: 30}}
		var seen areaLog
		return r.w.templateSparedLocked(&npcCast{npc: mage, target: target.ServerID, spell: sp}, r.w.templateAreaPlaceLocked(mage, sp, target.ServerID), target.ServerID, &seen)
	}
	if !isSpared(AreaOnCaster) || !isSpared(AreaOnTarget) {
		t.Errorf("its target behind a wall: spared %v round the caster, %v round the target", isSpared(AreaOnCaster), isSpared(AreaOnTarget))
	}
	if isSpared(AreaNone) {
		t.Error("a spell of no area spared its target")
	}
	r.w.ground = sightGround{}
	if isSpared(AreaOnCaster) || isSpared(AreaOnTarget) {
		t.Error("with nothing between them its target was spared")
	}
}

// A pet's area stops at a wall too.
func TestAWallStopsAPetsArea(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	mk := func(id EntityID, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: "Test Crab", Level: 40, HP: 5000, MaxHP: 5000, X: pet.X + 3 + dx, Y: pet.Y, Z: pet.Z, World: pet.World}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	target, near, hidden := mk(7101, 0), mk(7102, 4), mk(7103, 8)
	r.w.SetGround(sightGround{wallAt(pet.X + 3 + 6)})
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	takes := func(n *NPC) bool {
		return slices.Contains(r.w.petAreaFoesLocked(pet, target, SpellArea{Centre: AreaOnTarget, Radius: 30, Max: 10}, r.at(time.Second)), n)
	}
	if !takes(near) || takes(hidden) {
		t.Errorf("the pet's area takes the one on its side %v, the one behind the wall %v", takes(near), takes(hidden))
	}
}

// A thrown potion's splash stops at a wall; the one it hits takes its share.
func TestAWallStopsAThrownSplash(t *testing.T) {
	c := bodyRig(t)
	c.w.SpatialIndex.Insert(c.npc.ServerID, c.npc.X, c.npc.Z)
	behind := &NPC{ServerID: 3001, Name: "Rat", Level: 1, HP: 100, MaxHP: 100, X: c.npc.X + 6, Y: c.npc.Y, Z: c.npc.Z}
	c.w.NPCs[behind.ServerID] = behind
	c.w.SpatialIndex.Insert(behind.ServerID, behind.X, behind.Z)
	beside := &NPC{ServerID: 3002, Name: "Rat", Level: 1, HP: 100, MaxHP: 100, X: c.npc.X - 3, Y: c.npc.Y, Z: c.npc.Z}
	c.w.NPCs[beside.ServerID] = beside
	c.w.SpatialIndex.Insert(beside.ServerID, beside.X, beside.Z)
	c.w.SetGround(sightGround{wallAt(c.npc.X + 3)})
	pe := NewPotionEffect(9005, "Test Cocktail", "Target, Decrease Hitpoints by 20", "", 0, "", 0, 10)
	if r := c.use(t, pe, c.npcO, 0); r != "" {
		t.Fatalf("refused: %q", r)
	}
	if c.npc.HP != 1000-20 || beside.HP != 100-20 || behind.HP != 100 {
		t.Errorf("the one hit at %d, the one beside it at %d, the one behind the wall at %d", c.npc.HP, beside.HP, behind.HP)
	}
}

// A heal or a buff needs its caster to see the body (sight.go; petition 1243, Dock:
// "healing allows LOS through obstructions. do not allow heals through obstructions or
// buffs"): a heal on a friend behind a wall is refused before its power is spent, and
// lands on one this side of it (the control); a group's buff leaves out the member behind
// the wall; a friend who steps behind the wall during the cast is spared, its caster told.
// The other controls: harm on a creature behind the same wall begins as before, and with
// nothing in the way every member is reached.
func TestAHealNeedsItsCasterToSeeItsTarget(t *testing.T) {
	c := newCaster(t) // the caster at (100, 10, 100)
	c.w.SetGround(sightGround{wallAt(105)})
	behind := &Player{ServerID: 1002, Name: "Tank", HP: 100, MaxHP: 900, X: 110, Y: 10, Z: 100}
	near := &Player{ServerID: 1004, Name: "Near", HP: 100, MaxHP: 900, X: 103, Y: 10, Z: 101}
	c.w.Players[behind.ServerID], c.w.Players[near.ServerID] = behind, near
	vs := GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[1], vs.ChannelObjectID[1] = behind.ServerID, 0x1424A1D2
	vs.ChannelEntity[2], vs.ChannelObjectID[2] = near.ServerID, 0x1424A1D3
	heal := CastSpell{SpellID: 9101, Name: "Minor Healing", Scope: 1, Range: 30, CastTime: time.Second, PowerCost: 10,
		Def: ParseSpellNamed("Minor Healing", "Target, Increase Hitpoints by 50", "")}

	power := c.p.Power
	if c.w.StartCast(c.sess, heal, 0x1424A1D2, c.at(0)) {
		t.Fatal("a heal on a friend behind a wall began")
	}
	if lines := redLines(c.sess); c.p.Power != power || !slices.Contains(lines, castNotSeen) {
		t.Errorf("behind a wall: power %d (was %d), lines %q", c.p.Power, power, lines)
	}
	c.sent()

	c.land(t, heal, 0x1424A1D3, time.Minute)
	if near.HP != 150 {
		t.Errorf("the control, this side of the wall: HP %d, want 150", near.HP)
	}

	// stepping behind the wall during the cast
	if !c.w.StartCast(c.sess, heal, 0x1424A1D3, c.at(2*time.Minute)) {
		t.Fatal("the second heal did not begin")
	}
	near.X = 110
	c.tick(2*time.Minute + heal.CastTime + castLand)
	if lines := redLines(c.sess); near.HP != 150 || !slices.Contains(lines, castNotSeen) {
		t.Errorf("behind the wall as it lands: HP %d, lines %q", near.HP, lines)
	}
	c.sent()
	near.X = 103

	// a group's buff
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, behind.ServerID, near.ServerID}}}
	c.p.GroupID, behind.GroupID, near.GroupID = 7, 7, 7
	ward := CastSpell{SpellID: 9102, Name: "Group Ward", Scope: 2, Range: 30,
		Def: ParseSpellNamed("Group Ward", "Group, Increase AC by 10, 5min", "")}
	targets := func() ([]EntityID, string) {
		c.w.mu.Lock()
		defer c.w.mu.Unlock()
		return c.w.castTargetsLocked(c.sess, c.p, &ward, 0)
	}
	ids, refusal := targets()
	c.w.SetGround(nil)
	all, _ := targets()
	c.w.SetGround(sightGround{wallAt(105)})
	if refusal != "" || !slices.Equal(ids, []EntityID{c.p.ServerID, near.ServerID}) {
		t.Errorf("the group's buff reaches %v (%q); want the caster and the member in sight", ids, refusal)
	}
	if len(all) != 3 {
		t.Errorf("the control, nothing in the way: %v", all)
	}

	// harm on a creature behind the wall is refused before it begins (the
	// owner, 2026-11-13: no line of sight, no cast)
	c.npc.X = 110
	nuke := CastSpell{SpellID: 9103, Name: "Nuke", Scope: 1, Range: 30, CastTime: time.Second,
		Def: ParseSpellNamed("Nuke", "Target, Decrease Hitpoints by 10", "")}
	if c.w.StartCast(c.sess, nuke, c.npcO, c.at(3*time.Minute)) {
		t.Errorf("harm on a creature behind a wall began")
	}
}
