package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Area spells (area.go; jib's review, F3 pass 19), each rule beside its
// control: the lines' reading, the radius, the centre on the target or on the
// caster, the cap and the cast's 0x00B1, no player or pet hit, undead alone,
// the water's edge, a GM's row, a group's heal, and each hit the caster's.
// The lines are EQOA-Data's; the creatures and places are made up.

// areaRig is a level 50 wizard at (1000, 10, 1000), every stat 100, whose
// session sees each creature put beside it.
type areaRig struct {
	w    *World
	sess *session.Session
	p    *Player
	t0   time.Time
	me   uint32
	slot int
}

func newAreaRig(t *testing.T) *areaRig {
	t.Helper()
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 19), Port: 7000}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1001, Name: "Pyre", Class: 13, Level: 50, HP: 1500, MaxHP: 1500, Power: 9000, MaxPower: 9000,
		Str: 100, Sta: 100, Agi: 100, Dex: 100, Wis: 100, Int: 100, Cha: 100, Speed: 8.833333, X: 1000, Y: 10, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetPlayerObjectID(0x1919A001)
	return &areaRig{w: w, sess: sess, p: p, t0: time.Unix(1_800_000_000, 0), me: 0x1919A001}
}

// see gives id an entity channel of the caster's session, and returns the
// object id it knows it by.
func (r *areaRig) see(id EntityID) uint32 {
	vs := GetOrCreateVisibility(r.sess)
	o := 0x01010000 + uint32(id)
	vs.ChannelEntity[r.slot], vs.ChannelObjectID[r.slot] = id, o
	r.slot++
	return o
}

// creature is a level 40 creature of 5000 hit points at (x, 10, z), seen.
func (r *areaRig) creature(id EntityID, name string, x, z float32) *NPC {
	npc := &NPC{ServerID: id, Name: name, Level: 40, HP: 5000, MaxHP: 5000, X: x, Y: 10, Z: z,
		SpawnX: x, SpawnY: 10, SpawnZ: z, Size: 1}
	r.w.NPCs[id] = npc
	r.w.SpatialIndex.Insert(id, x, z)
	r.see(id)
	return npc
}

// player is another player at (x, 10, z), hurt, seen.
func (r *areaRig) player(id EntityID, name string, x, z float32) *Player {
	q := &Player{ServerID: id, Name: name, Level: 50, HP: 500, MaxHP: 1500, Power: 100, MaxPower: 1000, X: x, Y: 10, Z: z}
	r.w.Players[id] = q
	r.w.SpatialIndex.Insert(id, x, z)
	r.see(id)
	return q
}

// petOf gives q a summoned pet at (x, 10, z), hurt, seen.
func (r *areaRig) petOf(q *Player, id EntityID, x, z float32) *NPC {
	pet := r.creature(id, q.Name+"'s Golem", x, z)
	pet.Owner, pet.HP, pet.MaxHP = q.ServerID, 100, 1000
	if r.w.pets == nil {
		r.w.pets = make(map[EntityID]*petState)
	}
	r.w.pets[q.ServerID] = &petState{owner: q, npc: pet}
	return pet
}

func (r *areaRig) objectID(id EntityID) uint32 {
	if id == r.p.ServerID {
		return r.me
	}
	return r.w.objectIDSeenByLocked(r.sess, id)
}

func (r *areaRig) drain() [][]byte {
	var out [][]byte
	for _, m := range r.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// cast begins spell at id (0: no target) and lands it; it returns the start's
// 0x00B1.
func (r *areaRig) cast(t *testing.T, spell CastSpell, id EntityID) []byte {
	t.Helper()
	target := uint32(0xFFFFFFFF)
	if id != 0 {
		target = r.objectID(id)
	}
	r.p.Casting, r.p.Lockouts = nil, nil
	r.drain()
	if !r.w.StartCast(r.sess, spell, target, r.t0) {
		t.Fatalf("%q did not begin", spell.Name)
	}
	var b1 []byte
	for _, m := range r.drain() {
		if opOfMsg(m) == protocol.OpcodeCastingCombat {
			b1 = m
		}
	}
	r.w.mu.Lock()
	r.w.castSystem(r.t0.Add(spell.CastTime + castLand))
	r.w.mu.Unlock()
	r.drain()
	return b1
}

// spellOf is a spell as handlers/cast.go makes one: its line read with its
// name, its area read; scope as its record's, cast 2 s, range 30.
func spellOf(id, scope int, name, line string) CastSpell {
	d := ParseSpellNamed(name, line, "")
	return CastSpell{SpellID: id, DisplayIndex: id % 200, Level: 50, Scope: scope, Name: name, CastTime: 2 * time.Second,
		Recast: 3 * time.Second, PowerCost: 100, Range: 30, Def: d, Area: ReadSpellArea(name, line, d)}
}

// EQOA-Data's lines.
func fireStorm() CastSpell {
	return spellOf(700145, 1, "Fire Storm", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36")
}

func tauntingShout() CastSpell {
	return spellOf(700845, 1, "Taunting Shout", "Target, Increase Hate by 100 ; Increase 10% based on STR, AOE")
}

func turnUndead() CastSpell {
	return spellOf(700902, 1, "Turn Undead",
		"Target undead, Decrease HP by 160 ; Increase 30% based on WIS, AOE Decrease HP by 88 ; Increase 15% based on WIS")
}

func tideOfLife() CastSpell {
	return spellOf(700940, 2, "Tide of Life", "Group, Increase Hitpoints by 250 ; Increase 25% based on CHA, 15rad, 25hate")
}

// hits is how much each creature lost.
func hits(npcs ...*NPC) []int {
	out := make([]int, len(npcs))
	for i, n := range npcs {
		out[i] = n.MaxHP - n.HP
	}
	return out
}

func TestAreaSpellsAreReadFromTheirLines(t *testing.T) {
	hp := func(n int, s Stat, pct int) []SpellEffect {
		return []SpellEffect{{Kind: EffectHP, Amount: -n, AmountMax: -n, Scale: s, ScalePct: pct}}
	}
	for _, c := range []struct {
		name, line string
		centre     AreaCentre
		radius     float32 // radiusFor
		share      []SpellEffect
	}{
		{"Fire Storm", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36", AreaOnTarget, 15, hp(36, StatNone, 0)},
		{"Minor Fire Storm", "Target Decrease Hitpoints by 18 Aoe by 8", AreaOnTarget, 15, hp(8, StatNone, 0)},
		{"Minor Energy Storm", "Target Decrease Hitpoints by 38 AOE by Decrease Hitpoints by 16", AreaOnTarget, 15, hp(16, StatNone, 0)},
		{"Energy Storm", "Target, Decrease Hitpoints by 210 aoe Decrease Hitpoints by 90 - Lightning Based AoE", AreaOnTarget, 15, hp(90, StatNone, 0)},
		{"Greater Fire Storm", "Target, ecrease Hitpoints by 538 aoe Decrease Hitpoints by 230", AreaOnTarget, 15, hp(230, StatNone, 0)},
		// a scale after the area's figure is the spell's
		{"Fire Maelstrom", "Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36 ; Increase 30% based on DEX", AreaOnTarget, 15, hp(36, StatDex, 30)},
		{"Lightning Storm", "Target Debuff Hp by 690, aoe 390, Mod 15% based on DEX", AreaOnTarget, 15, hp(390, StatDex, 15)},
		// and one before it too
		{"Icy Fury", "Target, Decrease Hitpoints by 856 ; Increase 30% based on DEX aoe Decrease Hitpoints by 856", AreaOnTarget, 15, hp(856, StatDex, 30)},
		{"Thunderous Fury", "Target Debuff Hp by 856 Modified 30% based on DEX, AE 856", AreaOnTarget, 15, hp(856, StatDex, 30)},
		// a share with its own keeps it
		{"Turn Undead", "Target undead, Decrease HP by 160 ; Increase 30% based on WIS, AOE Decrease HP by 88 ; Increase 15% based on WIS", AreaOnTarget, 15, hp(88, StatWis, 15)},
		// by name: its storm twin's 360
		{"Lightning Maelstrom", "Target Debuff Hp by 846 Modified 30% based on DEX", AreaOnTarget, 15, hp(360, StatDex, 30)},
		// the shouts: round the caster, the whole spell
		{"Taunting Shout", "Target, Increase Hate by 100 ; Increase 10% based on STR, AOE", AreaOnCaster, 10, nil},
		{"Provoking Shout", "Increase Hate by 450 ; Increase 10% based on STR, AOE", AreaOnCaster, 10, nil},
		// the group heals: their lines' radius, else 15 by name
		{"Tide of Recovery", "Group, buff Hp by 63 Modified 25% based on CHA, Radius 15", AreaOnCaster, 15, nil},
		{"Wave of Life", "Group, Increase Hitpoints by 250 ; Increase 25% based on CHA, 15rad no hate", AreaOnCaster, 15, nil},
		{"Group Valor", "Group, Increase Maximum HP by 390, Increase AC by 390, 30min, 30rad, 70hate", AreaOnCaster, 30, nil},
		{"Tide of Glory", "Group, Buff Hp by 550 Modified 25% based on CHA", AreaOnCaster, 15, nil},
		// a GM's words
		{"Searing Nova", "Self, Decrease Hitpoints by 100, PB AE", AreaOnCaster, 10, nil},
		{"Pall", "Target, Decrease STR by 10, 1min, around the target", AreaOnTarget, 15, nil},
		// the controls: one target
		{"Static Bolt", "Target, Decrease Hitpoints by 70 ; Increase 30% based on DEX", AreaNone, 0, nil},
		{"Valor", "Target, Increase Maximum HP by 390, Increase AC by 390, 30min, 30rad, 293hate", AreaNone, 0, nil},
		{"Call to Battle", "Group, Increase AGI/STA by 35, Increase MaxHP by 320 ; Increase 5% based on WIS, 2min", AreaNone, 0, nil},
		{"Blood Storm", "Target, Increase Group HP by 375", AreaNone, 0, nil},
		{"Rampage", "Target, Decrease HP by 1100 ; Increase 20% based on STR, Increase Hate by 5 ; Increase 500% based on STR", AreaNone, 0, nil},
		{"Severupidus' Cocktail of Torment", "Level 60 Epic Cocktail path. Power cost: 140, Cast: 1, Recast: 1. 60 epic: 1800 AoE FR damage + wis mod", AreaNone, 0, nil},
	} {
		d := ParseSpellNamed(c.name, c.line, "")
		a := ReadSpellArea(c.name, c.line, d)
		if a.Centre != c.centre {
			t.Errorf("%s: centre %d, want %d (%+v)", c.name, a.Centre, c.centre, a)
			continue
		}
		if c.centre == AreaNone {
			continue
		}
		if r := a.radiusFor(d.Harmful); r != c.radius {
			t.Errorf("%s: radius %v, want %v", c.name, r, c.radius)
		}
		if len(a.Effects) != len(c.share) {
			t.Errorf("%s: the others take %+v, want %+v", c.name, a.Effects, c.share)
			continue
		}
		for i := range c.share {
			if a.Effects[i] != c.share[i] {
				t.Errorf("%s: the others take %+v, want %+v", c.name, a.Effects[i], c.share[i])
			}
		}
	}
}

// The target's share keeps its own scale: the Undead line's 30% WIS, which was
// read as the rest's 15%; the control, the line without its area's words, reads
// the same. A scale after the area's figure is still the target's when it has
// none (the maelstroms), and the area's words are no longer left unread.
func TestTheTargetKeepsItsOwnShare(t *testing.T) {
	whole := ParseSpellText("Target undead, Decrease HP by 160 ; Increase 30% based on WIS, AOE Decrease HP by 88 ; Increase 15% based on WIS")
	alone := ParseSpellText("Target undead, Decrease HP by 160 ; Increase 30% based on WIS")
	if len(whole.Effects) != 1 || whole.Effects[0] != alone.Effects[0] || whole.Effects[0].ScalePct != 30 {
		t.Errorf("Turn Undead's target: %+v, want %+v", whole.Effects, alone.Effects)
	}
	if d := ParseSpellText("Target, Decrease Hitpoints by 82, aoe Decrease Hitpoints by 36 ; Increase 30% based on DEX"); len(d.Effects) != 1 ||
		d.Effects[0].Amount != -82 || d.Effects[0].Scale != StatDex || d.Effects[0].ScalePct != 30 {
		t.Errorf("Fire Maelstrom's target: %+v", d.Effects)
	}
	for _, line := range []string{"Target, Increase Hate by 100 ; Increase 10% based on STR, AOE", "Target Debuff Hp by 856 Modified 30% based on DEX, AE 856"} {
		if d := ParseSpellText(line); len(d.Unread) != 0 || len(d.Effects) != 1 {
			t.Errorf("%q: unread %q, effects %+v", line, d.Unread, d.Effects)
		}
	}
	// Annihilate Undead's line says "Target," where its ranks' say "Target
	// undead"; its name makes it theirs. The control: a nuke not named so.
	if d := ParseSpellNamed("Annihilate Undead", "Target, Decrease Hp by 770 ; Increase 30% based on WIS, AOE Decrease Hp by 428 ; Increase 15% based on WIS", ""); !d.undeadOnly() {
		t.Error("Annihilate Undead works on the living")
	}
	if d := ParseSpellNamed("Annihilate", "Target, Decrease Hitpoints by 770", ""); d.undeadOnly() {
		t.Error("a nuke not named for the undead works on undead alone")
	}
}

// A storm lands round its target: a creature 14 from it takes the area's 36,
// one 16 from it nothing, one 5 from it across but 30 above it (on a cliff)
// nothing, and one beside the caster nothing (round the target, not the
// caster); the target takes its own 82. The control: the same cast as a spell
// of one target touches no one else.
func TestAStormHitsWhatIsRoundItsTarget(t *testing.T) {
	layout := func() (*areaRig, *NPC, *NPC, *NPC, *NPC, *NPC) {
		r := newAreaRig(t)
		target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
		in := r.creature(2002, "Deathfist Pawn", 1034, 1000)   // 14 from the target
		out := r.creature(2003, "Deathfist Pawn", 1036, 1000)  // 16 from it
		near := r.creature(2004, "Deathfist Pawn", 1001, 1000) // 1 from the caster, 19 from the target
		above := r.creature(2005, "Deathfist Pawn", 1025, 1000)
		above.Y = 40 // 5 across, 30 above
		return r, target, in, out, near, above
	}
	r, target, in, out, near, above := layout()
	r.cast(t, fireStorm(), target.ServerID)
	if got, want := hits(target, in, out, near, above), []int{82, 36, 0, 0, 0}; !equalInts(got, want) {
		t.Errorf("lost %v, want %v (the target, 14 off it, 16 off it, beside the caster, above it)", got, want)
	}
	if in.CurrentEnemy != r.p.ServerID {
		t.Errorf("the creature the storm hit hates %d, not its caster", in.CurrentEnemy)
	}

	r, target, in, _, _, _ = layout()
	one := fireStorm()
	one.Area = SpellArea{}
	r.cast(t, one, target.ServerID)
	if got := hits(target, in); got[0] != 82 || got[1] != 0 {
		t.Errorf("as a spell of one target: lost %v, want [82 0]", got)
	}
}

// A shout lands round its caster: the creature beside the caster takes its
// hate, one beside its target 18 off the caster none. The control: a storm in
// the same places takes the one beside its target and not the one beside the
// caster. A shout with no target word lands round its caster with no target.
func TestAShoutIsRoundItsCaster(t *testing.T) {
	layout := func() (*areaRig, *NPC, *NPC, *NPC) {
		r := newAreaRig(t)
		target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
		byCaster := r.creature(2002, "Deathfist Pawn", 1004, 1000) // 4 from the caster, 16 from the target
		byTarget := r.creature(2003, "Deathfist Pawn", 1018, 1003) // 3.6 from the target, 18.2 from the caster
		return r, target, byCaster, byTarget
	}
	r, target, byCaster, byTarget := layout()
	r.cast(t, tauntingShout(), target.ServerID)
	if target.CurrentEnemy != r.p.ServerID || byCaster.CurrentEnemy != r.p.ServerID {
		t.Errorf("the shout's target and the creature beside the caster hate %d and %d, want %d", target.CurrentEnemy, byCaster.CurrentEnemy, r.p.ServerID)
	}
	if byTarget.CurrentEnemy != 0 {
		t.Errorf("the creature beside the target, 18 off the caster, hates %d", byTarget.CurrentEnemy)
	}

	r, target, byCaster, byTarget = layout()
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(byCaster, byTarget); got[0] != 0 || got[1] != 36 {
		t.Errorf("the storm took %v from beside the caster and beside the target, want [0 36]", got)
	}

	r, _, byCaster, byTarget = layout()
	hp := r.p.HP
	r.cast(t, spellOf(700917, 0, "Provoking Shout", "Increase Hate by 450 ; Increase 10% based on STR, AOE"), 0)
	if byCaster.CurrentEnemy != r.p.ServerID || byTarget.CurrentEnemy != 0 || r.p.HP != hp || len(r.p.Effects) != 0 {
		t.Errorf("Provoking Shout with no target: beside the caster %d, beside the target %d, the caster %d HP and %d effects",
			byCaster.CurrentEnemy, byTarget.CurrentEnemy, r.p.HP, len(r.p.Effects))
	}
}

// The cap takes the nearest: of twelve creatures round its target the storm
// lands on the seven nearest and its target, and the cast's 0x00B1 names those
// seven, nearest first, in CLIENT's count and ids. With a GM's cap of 20 all
// twelve are hit, and the 0x00B1 still names seven, all CLIENT holds.
func TestTheCapTakesTheNearestAndTheCastNamesThem(t *testing.T) {
	layout := func() (*areaRig, *NPC, []*NPC) {
		r := newAreaRig(t)
		target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
		var ring []*NPC
		for k := 1; k <= 12; k++ {
			ring = append(ring, r.creature(EntityID(2100+k), "Deathfist Pawn", 1020, 1000+float32(k)))
		}
		return r, target, ring
	}
	r, target, ring := layout()
	b1 := r.cast(t, fireStorm(), target.ServerID)
	if target.HP != 5000-82 {
		t.Errorf("the target lost %d", 5000-target.HP)
	}
	for k, n := range ring {
		if want := map[bool]int{true: 36, false: 0}[k < 7]; 5000-n.HP != want {
			t.Errorf("the creature %d off its target lost %d, want %d", k+1, 5000-n.HP, want)
		}
	}
	// opcode, caster, target, the count, the ids, the cast time, the visual
	if len(b1) != 2+4+4+1+7*4+4+4 || b1[10] != 7 {
		t.Fatalf("the 0x00B1 % X: want seven others", b1)
	}
	for k := 0; k < 7; k++ {
		if got, want := binary.LittleEndian.Uint32(b1[11+4*k:]), r.objectID(ring[k].ServerID); got != want {
			t.Errorf("the 0x00B1's other %d is %#x, want %#x (the creature %d off its target)", k, got, want, k+1)
		}
	}

	r, target, ring = layout()
	wide := fireStorm()
	wide.Area = wide.Area.With(AreaRule{Max: 20})
	b1 = r.cast(t, wide, target.ServerID)
	for k, n := range ring {
		if n.HP != 5000-36 {
			t.Errorf("with a cap of 20, the creature %d off its target lost %d", k+1, 5000-n.HP)
		}
	}
	if b1[10] != 7 {
		t.Errorf("with a cap of 20 the 0x00B1 names %d others, more than CLIENT holds", b1[10])
	}
}

// The wire's own guard: a 0x00B1 never names more than seven, whoever asks.
func TestACastNamesSevenOthersAtMost(t *testing.T) {
	r := newAreaRig(t)
	r.w.sendCastingCombat(r.sess, 1, 2, 3000, 0xE78024D9, 10, 11, 12, 13, 14, 15, 16, 17, 18)
	msgs := r.drain()
	if len(msgs) != 1 || len(msgs[0]) != 2+4+4+1+7*4+4+4 || msgs[0][10] != 7 {
		t.Fatalf("% X", msgs)
	}
	r.w.sendCastingCombat(r.sess, 1, 2, 3000, 0xE78024D9)
	if msgs = r.drain(); len(msgs[0]) != 2+4+4+1+4+4 || msgs[0][10] != 0 {
		t.Errorf("one target's 0x00B1: % X", msgs[0])
	}
}

// An area's harm lands on creatures only: not another player (the caster's
// group either), a player's pet, a charmed creature, an NPC no one may attack,
// a corpse, nor a creature of another world; the control, a creature among
// them, is hit.
func TestAnAreaSparesPlayersAndPets(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
	friend := r.player(1002, "Ally", 1022, 1000)
	r.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{r.p.ServerID, friend.ServerID}}}
	r.p.GroupID, friend.GroupID = 7, 7
	pet := r.petOf(friend, 2002, 1023, 1000)
	pet.HP = pet.MaxHP
	charmed := r.creature(2003, "Deathfist Pawn", 1024, 1000)
	charmed.Owner = r.p.ServerID
	townsfolk := r.creature(2004, "Merchant Gale", 1025, 1000)
	townsfolk.NPCType = 0x80
	corpse := r.creature(2005, "Deathfist Pawn", 1026, 1000)
	corpse.Dead = true
	elsewhere := r.creature(2006, "Deathfist Pawn", 1021, 1001)
	elsewhere.World = 1
	control := r.creature(2007, "Deathfist Pawn", 1027, 1000)

	b1 := r.cast(t, fireStorm(), target.ServerID)
	if friend.HP != 500 {
		t.Errorf("the caster's groupmate lost %d", 500-friend.HP)
	}
	// and the cast names the creature alone among its others
	if len(b1) < 15 || b1[10] != 1 || binary.LittleEndian.Uint32(b1[11:]) != r.objectID(control.ServerID) {
		t.Errorf("the 0x00B1 % X: want the one creature", b1)
	}
	if got := hits(pet, charmed, townsfolk, corpse, elsewhere); !equalInts(got, make([]int, 5)) {
		t.Errorf("the pet, the charmed, the townsfolk, the corpse and the other world's lost %v", got)
	}
	if got := hits(control); got[0] != 36 {
		t.Errorf("the creature among them lost %d, want 36", got[0])
	}
}

// Each of an area's others takes its share at the landing only if it is still
// there: one dead since the cast began, or charmed since, takes nothing; the
// control, one still there, takes its 36.
func TestAnAreasOtherMustStillBeThere(t *testing.T) {
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
	died := r.creature(2002, "Deathfist Pawn", 1022, 1000)
	charmed := r.creature(2003, "Deathfist Pawn", 1024, 1000)
	there := r.creature(2004, "Deathfist Pawn", 1026, 1000)
	spell := fireStorm()
	if !r.w.StartCast(r.sess, spell, r.objectID(target.ServerID), r.t0) {
		t.Fatal("not begun")
	}
	died.Dead, died.HP = true, 0
	charmed.Owner = r.p.ServerID
	r.w.mu.Lock()
	r.w.castSystem(r.t0.Add(spell.CastTime + castLand))
	r.w.mu.Unlock()
	if got := hits(charmed, there); got[0] != 0 || got[1] != 36 {
		t.Errorf("the charmed since and the one still there lost %v, want [0 36]", got)
	}
	if died.HP != 0 {
		t.Errorf("the dead took the hit: HP %d", died.HP)
	}
}

// The Undead line lands on undead alone: Turn Undead at a skeleton takes the
// skeleton beside it (88 and 15% of WIS 100) and not the orc beside it, and
// the target its own 160 and 30% of WIS. The control: a storm takes the orc.
func TestTheUndeadLineHitsOnlyUndead(t *testing.T) {
	layout := func() (*areaRig, *NPC, *NPC, *NPC) {
		r := newAreaRig(t)
		return r, r.creature(2001, "a decaying skeleton", 1020, 1000),
			r.creature(2002, "a decaying skeleton", 1025, 1000), r.creature(2003, "Deathfist Pawn", 1024, 1002)
	}
	r, target, bones, orc := layout()
	r.cast(t, turnUndead(), target.ServerID)
	if got, want := hits(target, bones, orc), []int{160 + 30, 88 + 15, 0}; !equalInts(got, want) {
		t.Errorf("lost %v, want %v (the target, the skeleton beside it, the orc)", got, want)
	}

	r, target, _, orc = layout()
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(orc); got[0] != 36 {
		t.Errorf("a storm took %d from the orc, want 36", got[0])
	}
}

// An area stops at the water's edge: a storm cast on the shore at a creature on
// the shore takes nothing in the water beside it. The control: with no water
// known, it does.
func TestAnAreaStopsAtTheWatersEdge(t *testing.T) {
	layout := func(water bool) (*areaRig, *NPC, *NPC) {
		r := newAreaRig(t)
		if water {
			r.w.SetEnv(seaWater{}) // water below 50 (water_test.go)
		}
		r.p.Y = 51
		target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
		target.Y = 51
		swimmer := r.creature(2002, "Deathfist Pawn", 1025, 1000)
		swimmer.Y = 49 // in the water, 5.4 from the target
		return r, target, swimmer
	}
	r, target, swimmer := layout(true)
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(target, swimmer); got[0] != 82 || got[1] != 0 {
		t.Errorf("with the water known: lost %v, want [82 0]", got)
	}
	r, target, swimmer = layout(false)
	r.cast(t, fireStorm(), target.ServerID)
	if got := hits(swimmer); got[0] != 36 {
		t.Errorf("with no water known the creature beside the target lost %d, want 36", got[0])
	}
}

// A GM's row (table spellarea) over the line's reading: a radius of 30 takes
// the creature 25 from its target (the line's 15 does not), a cap of 2 takes
// its target and the nearest, "not an area spell" its target alone, and "round
// its caster" makes a nuke of one target a blast round its caster (10 off it,
// not 12).
func TestAGMsRowOverridesTheArea(t *testing.T) {
	layout := func() (*areaRig, *NPC, *NPC, *NPC) {
		r := newAreaRig(t)
		return r, r.creature(2001, "Deathfist Pawn", 1020, 1000),
			r.creature(2002, "Deathfist Pawn", 1020, 1005), r.creature(2003, "Deathfist Pawn", 1020, 1025)
	}
	for _, c := range []struct {
		name string
		rule *AreaRule
		want []int // the target, 5 off it, 25 off it
	}{
		{"the line's", nil, []int{82, 36, 0}},
		{"a radius of 30", &AreaRule{Radius: 30}, []int{82, 36, 36}},
		{"a cap of 2, a radius of 30", &AreaRule{Radius: 30, Max: 2}, []int{82, 36, 0}},
		{"not an area spell", &AreaRule{Centre: AreaRuleNone, Radius: 30}, []int{82, 0, 0}},
	} {
		r, target, near, far := layout()
		spell := fireStorm()
		if c.rule != nil {
			spell.Area = spell.Area.With(*c.rule)
		}
		r.cast(t, spell, target.ServerID)
		if got := hits(target, near, far); !equalInts(got, c.want) {
			t.Errorf("%s: lost %v, want %v", c.name, got, c.want)
		}
	}

	nuke := spellOf(650, 1, "Searing Bolt", "Target, Decrease Hitpoints by 100")
	r := newAreaRig(t)
	target := r.creature(2001, "Deathfist Pawn", 1020, 1000)
	close := r.creature(2002, "Deathfist Pawn", 1008, 1000) // 8 from the caster
	far := r.creature(2003, "Deathfist Pawn", 1012, 1000)   // 12 from it
	r.cast(t, nuke, target.ServerID)
	if got := hits(target, close, far); !equalInts(got, []int{100, 0, 0}) {
		t.Errorf("the nuke without a row: lost %v", got)
	}
	r = newAreaRig(t)
	target = r.creature(2001, "Deathfist Pawn", 1020, 1000)
	close = r.creature(2002, "Deathfist Pawn", 1008, 1000)
	far = r.creature(2003, "Deathfist Pawn", 1012, 1000)
	nuke.Area = nuke.Area.With(AreaRule{Centre: AreaRuleCaster})
	r.cast(t, nuke, target.ServerID)
	if got := hits(target, close, far); !equalInts(got, []int{100, 100, 0}) {
		t.Errorf("the nuke made a blast round its caster: lost %v, want [100 100 0]", got)
	}
}

// A group's heal lands on its group within its line's radius (15) and on their
// pets: a member 10 off and its pet 12 off are healed, a member 20 off and a
// stranger 5 off are not, and the cast's 0x00B1 names the member and the pet.
// The control: a group heal whose line gives no radius reaches the member 20
// off (its range, 30, as before) and no pet.
func TestAGroupHealTakesItsGroupAndTheirPets(t *testing.T) {
	layout := func() (*areaRig, *Player, *Player, *Player, *NPC) {
		r := newAreaRig(t)
		r.p.Class, r.p.HP = 9, 1000
		near := r.player(1002, "Near", 1010, 1000)
		far := r.player(1003, "Far", 1020, 1000)
		stranger := r.player(1004, "Stranger", 1005, 1000)
		r.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{r.p.ServerID, near.ServerID, far.ServerID}}}
		r.p.GroupID, near.GroupID, far.GroupID = 7, 7, 7
		return r, near, far, stranger, r.petOf(near, 2001, 1012, 1000)
	}
	heal := 250 + 25 // and 25% of CHA 100
	r, near, far, stranger, pet := layout()
	b1 := r.cast(t, tideOfLife(), r.p.ServerID)
	if r.p.HP != 1000+heal || near.HP != 500+heal || far.HP != 500 || stranger.HP != 500 || pet.HP != 100+heal {
		t.Errorf("healed: the caster to %d, 10 off to %d, 20 off to %d, a stranger to %d, the pet to %d", r.p.HP, near.HP, far.HP, stranger.HP, pet.HP)
	}
	if len(b1) < 11 || b1[10] != 2 || binary.LittleEndian.Uint32(b1[11:]) != r.objectID(near.ServerID) ||
		binary.LittleEndian.Uint32(b1[15:]) != r.objectID(pet.ServerID) {
		t.Errorf("the 0x00B1 % X: want the member and its pet", b1)
	}

	r, near, far, _, pet = layout()
	plain := spellOf(9001, 2, "Group Mending", "Group, Increase Hitpoints by 250 ; Increase 25% based on CHA")
	if plain.Area.Centre != AreaNone {
		t.Fatalf("a group heal with no radius read as an area: %+v", plain.Area)
	}
	r.cast(t, plain, r.p.ServerID)
	if near.HP != 500+heal || far.HP != 500+heal || pet.HP != 100 {
		t.Errorf("the plain group heal: 10 off to %d, 20 off to %d, the pet to %d", near.HP, far.HP, pet.HP)
	}
}

// Each of an area's hits is the caster's on it: a creature the storm splashed
// calls the helpers of its faction (socialaggro.go), one 26 from it and 40 from
// the storm's target coming. The control: with no creature of its faction in
// the storm, the helper stays.
func TestEachOfAnAreasHitsCallsItsHelpers(t *testing.T) {
	layout := func(splashed bool) (*areaRig, *NPC) {
		r := newAreaRig(t)
		r.w.SetSocialRules(map[string]SocialRule{"Deathfist Pawn": {Social: true, Faction: "Deathfist Orcs"}})
		target := r.creature(2001, "Dervish Cutthroat", 1020, 1000)
		if splashed {
			r.creature(2002, "Deathfist Pawn", 1034, 1000) // in the storm, 14 from its target
		}
		helper := r.creature(2003, "Deathfist Pawn", 1060, 1000) // 26 from it, 40 from the target
		r.cast(t, fireStorm(), target.ServerID)
		return r, helper
	}
	if r, helper := layout(true); helper.CurrentEnemy != r.p.ServerID {
		t.Errorf("the helper of the splashed creature's faction fights %d, not the caster", helper.CurrentEnemy)
	}
	if _, helper := layout(false); helper.CurrentEnemy != 0 {
		t.Errorf("with no creature of its faction in the storm the helper fights %d", helper.CurrentEnemy)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
