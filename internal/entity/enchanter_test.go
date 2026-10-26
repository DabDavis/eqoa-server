package entity

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The enchanter's lines (enchanter.go), as eqoaclassic's records have them:
// the imported ones carry their line as their description, so nothing but
// their names says they are procs.

// The proc lines by name: the Anarchic ranks (the group's), Chaotic Rune and
// Stormlord's Rune are procs, no longer harmful, the group's still the
// group's, a line's scale kept. The controls: the Chaotic nuke (no duration)
// stays a nuke, and the same group line under a name of no proc family stays
// a harmful hit.
func TestTheEnchantersProcsGoByTheirNames(t *testing.T) {
	for _, c := range []struct {
		name, line string
		proc       bool
		target     SpellTarget
		amount     int
		scale      Stat
		pct        int
	}{
		{"Anarchic Flux", "Group, Decrease Hitpoints by 64, 5min", true, TargetGroup, 64, StatNone, 0},
		{"Anarchic Rune", "Group, Decrease Hitpoints by 256, 5min", true, TargetGroup, 256, StatNone, 0},
		{"Chaotic Rune", "Target, Decrease Hitpoints by 256, 5min", true, TargetSingle, 256, StatNone, 0},
		{"Stormlord's Rune", "Group, Decrease Hitpoints by 336 ; Increase 10% based on DEX ,5min", true, TargetGroup, 336, StatDex, 10},
		{"Rune of Animation", "Target, Decrease Hitpoints by 336 ; Increase 10% based on INT, 30min", true, TargetSingle, 336, StatInt, 10},
		// the controls
		{"Chaotic Feedback", "Target, Decrease Hitpoints by 74 ; Increase 10% based on DEX", false, TargetSingle, -74, StatDex, 10},
		{"Thunderous Wave", "Group, Decrease Hitpoints by 64, 5min", false, TargetGroup, -64, StatNone, 0},
	} {
		d := ParseSpellNamed(c.name, c.line, c.line)
		if len(d.Effects) != 1 {
			t.Fatalf("%s: %+v", c.name, d)
		}
		e := d.Effects[0]
		if c.proc {
			if e.Kind != EffectProc || d.Harmful || d.Target != c.target || e.Amount != c.amount || e.Scale != c.scale || e.ScalePct != c.pct {
				t.Errorf("%s: %+v, harmful %v, target %d; want a proc of %d", c.name, e, d.Harmful, d.Target, c.amount)
			}
			continue
		}
		if e.Kind != EffectHP || !d.Harmful || e.Amount != c.amount {
			t.Errorf("%s (control): %+v, harmful %v; want a hit of %d", c.name, e, d.Harmful, c.amount)
		}
	}
}

// A proc's scale is its caster's at its landing: Stormlord's Rune from a
// caster of DEX 200 carries 336 + 20. The control: Chaotic Rune, no scale,
// carries its 256 whatever the caster's stats.
func TestAProcsScaleIsItsCasters(t *testing.T) {
	c := newCaster(t)
	c.p.Dex = 200
	now := c.at(0)
	rune := CastSpell{SpellID: 701232, Name: "Stormlord's Rune", Scope: 2,
		Def: ParseSpellNamed("Stormlord's Rune", "Group, Decrease Hitpoints by 336 ; Increase 10% based on DEX ,5min", "")}
	if ae := lasting(&castState{spell: rune}, c.p, now); ae == nil || ae.Proc != 356 || ae.ProcMax != 356 || ae.Until != now.Add(5*time.Minute) {
		t.Errorf("Stormlord's Rune: %+v; want a proc of 356 for five minutes", ae)
	}
	chaotic := CastSpell{SpellID: 700708, Name: "Chaotic Rune", Scope: 1,
		Def: ParseSpellNamed("Chaotic Rune", "Target, Decrease Hitpoints by 256, 5min", "")}
	if ae := lasting(&castState{spell: chaotic}, c.p, now); ae == nil || ae.Proc != 256 {
		t.Errorf("Chaotic Rune (control): %+v; want 256", ae)
	}
}

// groupRig is the caster in a group with a friend, each with a pet beside
// them, and a third member far off.
func groupRig(t *testing.T) (*caster, *Player, *NPC, *NPC) {
	c := newCaster(t)
	c.p.Class = classEnchanter
	friend := &Player{ServerID: 1002, Name: "Tank", HP: 900, MaxHP: 900, X: 110, Y: 10, Z: 100}
	far := &Player{ServerID: 1003, Name: "Far", HP: 900, MaxHP: 900, X: 900, Y: 10, Z: 100}
	c.w.Players[friend.ServerID], c.w.Players[far.ServerID] = friend, far
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, friend.ServerID, far.ServerID}}}
	c.p.GroupID, friend.GroupID, far.GroupID = 7, 7, 7
	mine := &NPC{ServerID: 4001, Name: "Animation", Level: 20, HP: 400, MaxHP: 400, Owner: c.p.ServerID, X: 101, Y: 10, Z: 100}
	his := &NPC{ServerID: 4002, Name: "Skeleton", Level: 20, HP: 400, MaxHP: 400, Owner: friend.ServerID, X: 111, Y: 10, Z: 100}
	c.w.NPCs[mine.ServerID], c.w.NPCs[his.ServerID] = mine, his
	c.w.pets = map[EntityID]*petState{c.p.ServerID: {owner: c.p, npc: mine}, friend.ServerID: {owner: friend, npc: his}}
	return c, friend, mine, his
}

// The group's proc lands on the members in range and on their pets; the
// control: the group's power over time (no proc) reaches no pet.
func TestAGroupProcReachesTheMembersPets(t *testing.T) {
	c, friend, mine, his := groupRig(t)
	flux := CastSpell{SpellID: 700876, Name: "Anarchic Flux", Scope: 2, Range: 30,
		Def: ParseSpellNamed("Anarchic Flux", "Group, Decrease Hitpoints by 64, 5min", "")}
	c.w.mu.Lock()
	ids, refusal := c.w.castTargetsLocked(c.sess, c.p, &flux, 0)
	c.w.mu.Unlock()
	want := map[EntityID]bool{c.p.ServerID: true, friend.ServerID: true, mine.ServerID: true, his.ServerID: true}
	if refusal != "" || len(ids) != len(want) {
		t.Fatalf("Anarchic Flux's targets %v (%q); want %v", ids, refusal, want)
	}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("Anarchic Flux lands on %d", id)
		}
	}
	c.land(t, flux, 0, 0)
	if len(his.Effects) != 1 || his.Effects[0].Proc != 64 || len(friend.Effects) != 1 || friend.Effects[0].Proc != 64 {
		t.Errorf("after it lands: the friend's pet %+v, the friend %+v", his.Effects, friend.Effects)
	}

	breeze := CastSpell{SpellID: 700861, Name: "Empathic Breeze", Scope: 2, Range: 30,
		Def: ParseSpellNamed("Empathic Breeze", "Group, Increase Power per tic by 5, 15min", "")}
	c.w.mu.Lock()
	ids, _ = c.w.castTargetsLocked(c.sess, c.p, &breeze, 0)
	c.w.mu.Unlock()
	if len(ids) != 2 {
		t.Errorf("Empathic Breeze (control) lands on %v; want the caster and the friend alone", ids)
	}
}

func clarityLine(id int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 5, Level: 34, Scope: 1, Name: name, CastTime: 2 * time.Second, Recast: 3 * time.Second,
		Range: 30, Def: ParseSpellNamed(name, line, "")}
}

// The clarity line is one at a time: Breeze (5) over Clarity (17) does
// nothing and the caster is told; Empathy (33) replaces Clarity with no tick
// at once; the controls: Mana Flow, not of the line, goes on beside Empathy,
// and with nothing of the line on, Clarity ticks at its landing.
func TestTheClarityLineIsOneAtATime(t *testing.T) {
	c := newCaster(t)
	c.p.Power, c.p.MaxPower = 100, 5000
	c.land(t, clarityLine(561, "Clarity", "Target, Increase Power per tic by 17, 15min"), c.me, 0)
	if len(c.p.Effects) != 1 || c.p.Power != 117 {
		t.Fatalf("Clarity: effects %d, power %d; want one and 117 (its tick at its landing)", len(c.p.Effects), c.p.Power)
	}
	if !c.w.StartCast(c.sess, clarityLine(512, "Breeze", "Target, Increase Power per tic by 5, 15min"), c.me, c.at(10*time.Second)) {
		t.Fatal("Breeze did not begin")
	}
	c.sent()
	c.tick(10*time.Second + 2*time.Second + castLand)
	told := false
	for _, m := range c.sent() {
		told = told || bytes.Contains(m, wide16("already has a stronger power over time"))
	}
	if len(c.p.Effects) != 1 || c.p.Effects[0].Name != "Clarity" || !told {
		t.Errorf("Breeze over Clarity: effects %+v, told %v; want Clarity alone, the caster told", c.p.Effects, told)
	}
	power := c.p.Power
	c.land(t, clarityLine(700779, "Empathy", "Target, Increase Power per tic by 33, 15min"), c.me, 20*time.Second)
	if len(c.p.Effects) != 1 || c.p.Effects[0].Name != "Empathy" || c.p.Power != power {
		t.Errorf("Empathy over Clarity: effects %+v, power %d (was %d); want Empathy alone, no tick at once", c.p.Effects, c.p.Power, power)
	}
	c.land(t, clarityLine(700755, "Mana Flow", "Group, Increase Power per tic by 200 ; Increase 50% based on CHA, 24sec"), c.me, 30*time.Second)
	if len(c.p.Effects) != 2 {
		t.Errorf("Mana Flow (control) beside Empathy: %+v", c.p.Effects)
	}
}

// An arcane resist debuff takes its share off a creature's resist rolls:
// Paralytic Punishment (25 AR) 6 points, worn off none; a proc the creature
// resisted at par (a roll of 4 under 5) gets through under it; a charm the
// creature resisted (a roll of 5 under 10) takes. The controls: no debuff,
// no points off, and the same rolls resisted.
func TestAnArcaneDebuffLowersACreaturesResists(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	npc := &NPC{ServerID: 9, Level: 30}
	if off := npcResistOffPct(npc, now); off != 0 {
		t.Fatalf("no debuff (control): %d off", off)
	}
	npc.Effects = []*ActiveEffect{{SpellID: 701198, Until: now.Add(15 * time.Minute), Mods: StatMods{AR: -25}}}
	if off := npcResistOffPct(npc, now); off != 6 {
		t.Errorf("Paralytic Punishment: %d off; want 6", off)
	}
	if off := npcResistOffPct(npc, now.Add(16*time.Minute)); off != 0 {
		t.Errorf("worn off: %d off", off)
	}

	effects := []*ActiveEffect{{Proc: 64, ProcMax: 64, Until: now.Add(time.Minute)}}
	if hits, resisted := procRolls(effects, 100, 30, 30, 0, now, dice(0, 4)); len(hits) != 0 || resisted != 1 {
		t.Errorf("a proc at par, no debuff (control): hits %v, resisted %d", hits, resisted)
	}
	if hits, resisted := procRolls(effects, 100, 30, 30, 6, now, dice(0, 4)); len(hits) != 1 || resisted != 0 {
		t.Errorf("a proc under Paralytic Punishment: hits %v, resisted %d", hits, resisted)
	}

	charm := func(debuff bool) bool {
		r, _, _ := charmRig(t)
		r.p.Cha = 0
		crab := r.enemy
		if debuff {
			crab.Effects = []*ActiveEffect{{SpellID: 701198, Until: r.at(time.Hour), Mods: StatMods{AR: -25}}}
		}
		was := charmIntn
		charmIntn = func(int) int { return 5 }
		defer func() { charmIntn = was }()
		castCharm(t, r, captivation(), crabID, 0)
		return crab.Owner == r.p.ServerID
	}
	if charm(false) {
		t.Error("a charm rolled 5 under 10 took with no debuff (control)")
	}
	if !charm(true) {
		t.Error("a charm rolled 5 did not take under Paralytic Punishment (10 - 6)")
	}
}

// Earthen Golem wears a granite golem's body and Ferrous Golem an iron
// golem's, as no NPC of their names exists; the control: Gymon's Animation
// is the animation it was.
func TestTheGolemsWearGolems(t *testing.T) {
	w := NewWorld()
	w.SessionManager = session.NewManager()
	_ = net.IPv4zero
	for _, n := range []*NPC{
		{ServerID: 1, Name: "granite golem", ModelID: 0x111, Size: 1},
		{ServerID: 2, Name: "Iron Golem", ModelID: 0x222, Size: 1},
		{ServerID: 3, Name: "Animation", ModelID: 0x333, Size: 1},
	} {
		w.NPCs[n.ServerID] = n
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range []struct {
		spell string
		model int64
	}{{"Earthen Golem", 0x111}, {"Ferrous Golem", 0x222}, {"Gymon's Animation", 0x333}} {
		if tm := w.petTemplateLocked(c.spell, "", classEnchanter); tm == nil || tm.ModelID != c.model {
			t.Errorf("%s: %+v; want the body %#x", c.spell, tm, c.model)
		}
	}
}

// The clarity line by name: both bars' six ranks, the group's, a "+" rank;
// the controls: the bursts, the bards' songs and a potion's name are not of
// it.
func TestTheClarityLineByName(t *testing.T) {
	for _, n := range []string{"Breeze", "Intuition", "Clarity", "Lucidity", "Tranquility", "Empathy",
		"Empathic Breeze", "Empathic Intuition", "Empathic Lucidity", "Empathic Tranquility", "Clarity+"} {
		if !clarityFamily(n) {
			t.Errorf("%s: not of the clarity line", n)
		}
	}
	for _, n := range []string{"Mana Flow", "Power Flow", "Power Flood", "Concerto", "Power Dance", "Clarity Potion"} {
		if clarityFamily(n) {
			t.Errorf("%s (control): of the clarity line", n)
		}
	}
}
