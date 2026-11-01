package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Only the best of a line counts on a creature (bestOfLine), as on a player:
// two ranks of one AC debuff's line on it take off what the higher takes off,
// not both; an effect of another line counts beside it.
func TestOnlyTheBestOfADebuffsLineCountsOnACreature(t *testing.T) {
	npc := &NPC{ServerID: 5, Name: "Desert Crab", Level: 30}
	until := time.Now().Add(time.Hour)
	npc.Effects = []*ActiveEffect{
		{SpellID: 1, Until: until, Line: "player -AC", Rank: 10, Mods: StatMods{AC: -60}},
		{SpellID: 2, Until: until, Line: "player -AC", Rank: 25, Mods: StatMods{AC: -150}},
		{SpellID: 3, Until: until, Line: "player -STR", Rank: 12, Mods: StatMods{Str: -20}},
	}
	if m := npc.creatureMods(); m.AC != -150 || m.Str != -20 {
		t.Errorf("AC %d, STR %d; want the higher rank's -150 and the other line's -20", m.AC, m.Str)
	}
}

// The simulator casts a book's pure debuffs (the user, 2026-11-01: "make the
// sim cast pure debuffs too"): a harmful spell at a target with no damage
// whose lasting part lowers a stat that counts in a creature's fights is a
// debuff, only the highest of a line in the book, cast whenever its line is
// not on the foe; one lowering STA alone (nothing on a creature) is not. A
// warrior that weakens its foe's STR in a long fight (simDebuffLook,
// simLongFight) ends it with more of its hit points than one that does not,
// on the same dice.
func TestTheSimCastsPureDebuffs(t *testing.T) {
	debuff := func(id, level int, name, line string) CastSpell {
		return CastSpell{SpellID: id, DisplayIndex: id % 40, Level: level, Scope: 1, Name: name, CastTime: time.Second,
			PowerCost: 10, Range: 20, Def: ParseSpellText(line)}
	}
	weak := debuff(9301, 10, "Test Rust", "Target, Decrease AC by 60, 30sec")
	strong := debuff(9302, 25, "Test Corrosion", "Target, Decrease AC by 150, 30sec")
	tired := debuff(9303, 25, "Test Weariness", "Target, Decrease STA by 50, 30sec")
	for _, c := range []struct {
		s    CastSpell
		want spellRole
	}{{weak, roleDebuff}, {strong, roleDebuff}, {tired, roleNone}} {
		if got := simRole(&c.s); got != c.want {
			t.Errorf("%q: role %d, want %d", c.s.Name, got, c.want)
		}
	}
	plain := simKit(0, 30)
	debuffing := simKit(0, 30)
	debuffing.Spells = append(debuffing.Spells, weak, strong, tired)
	f := newSimFight(debuffing, SimNPC{Level: 31}, SeedBalance())
	f.sortBook()
	if len(f.debuffs) != 1 || f.debuffs[0].SpellID != strong.SpellID {
		t.Fatalf("the book's debuffs %v; want Test Corrosion alone (the highest of its line; STA is no debuff)", f.debuffs)
	}
	weaken := simKit(0, 30)
	weaken.Spells = append(weaken.Spells, debuff(9304, 25, "Test Enfeebling", "Target, Decrease STR by 200, 2min"))
	foe := SimNPC{Level: 32}
	a := SimulateFight(weaken, foe, SeedBalance(), 5, 10*time.Minute)
	b := SimulateFight(plain, foe, SeedBalance(), 5, 10*time.Minute)
	if a.Debuffs == 0 || b.Debuffs != 0 {
		t.Fatalf("debuffs cast: %d with the spell, %d without", a.Debuffs, b.Debuffs)
	}
	if !a.Won || !b.Won || a.HP <= b.HP {
		t.Errorf("with the debuff: won %v with %d HP; without: won %v with %d; want both won, the debuffer with more", a.Won, a.HP, b.Won, b.HP)
	}
}

// A shadow knight's scream takes AC off its target and gives it to the
// knight (the user, 2026-11-01: "SK drain should land on themself by
// default, CM may allow group"): the workbook gives the share to the group,
// so it landed on the target with the rest, which took off nothing. A
// scroll's scream: the creature at -50, the knight at +50, the knight's
// group member beside it unchanged; a mastery's form (a rank of cmcatalog
// adds it): the member at +50 too.
func TestAScreamsACIsTheKnightsOrItsGroups(t *testing.T) {
	noResist(t)
	c := newCaster(t)
	c.p.Class, c.p.Level, c.p.AC = 3, 30, 100
	c.npc.Level, c.npc.HP, c.npc.MaxHP = 31, 5000, 5000
	friend := &Player{ServerID: 1002, Name: "Friend", Class: 0, Level: 30, HP: 900, MaxHP: 900, AC: 200, X: 103, Y: 10, Z: 100}
	c.w.Players[friend.ServerID] = friend
	c.p.GroupID, friend.GroupID = 7, 7
	c.w.groups = map[uint32]*Group{7: {ID: 7, Members: []EntityID{c.p.ServerID, friend.ServerID}}}
	scream := func(id int, name string) CastSpell {
		sp := wbSpell(id, 25, 1, name, `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":600,"stack":0,"level":25,"radius":30,
			"effects":[{"id":"0001","type":"Statistic Change","kind":"stat","stat":"ac","amount":-50},
				{"id":"0002","type":"Statistic Change","kind":"stat","stat":"ac","amount":50,"scope":"group","target_type":"Friendly"}]}`)
		sp.Recast = 0
		return sp
	}
	c.land(t, scream(9401, "Test Scream"), c.npcO, 0)
	if m := c.npc.creatureMods(); m.AC != -50 {
		t.Errorf("the creature's AC %+d; want -50 (the group's +50 not on it)", m.AC)
	}
	if got := c.p.StatNow(StatAC); got != 150 {
		t.Errorf("the knight's AC %d; want 150, its 100 and the scream's 50", got)
	}
	if got := friend.StatNow(StatAC); got != 200 {
		t.Errorf("the group member's AC %d; want its own 200 (a scroll's scream is the knight's)", got)
	}

	c.w.SetCMCatalog([]database.CMRankRow{{ID: 1, Category: 2, Classes: 8, Races: 2047, Mastery: "Test Screams", Rank: 1,
		Name: "Test Scream II", Adds: "Test Scream II@25", AddSpells: "9402", Enabled: true}})
	c.land(t, scream(9402, "Test Scream II"), c.npcO, 20*time.Second)
	if got := friend.StatNow(StatAC); got != 250 {
		t.Errorf("the group member's AC under a mastery's scream %d; want 250", got)
	}
}

// A necromancer's pet's drain of its enemy's stats (the workbook's Undead
// Bane, from the level 30 pet; the user, 2026-11-01: "necromancers mummy and
// higher pets have a drain"): STR 30 (and a tenth of the pet's own) and AC 90
// taken off the creature, which counts in its fights (creatureMods), and the
// pet's for the spell's minute (effectMods); a second cast replaces the
// first. A pet whose template has a life tap of its own (Undead Blood) does
// not also tap with its owner's spell (ownsTap); one without does.
func TestANecromancersPetDrainsItsEnemysStats(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	r.p.Class = classNecromancer
	ps := r.w.pets[r.p.ServerID]
	if ps == nil {
		t.Fatal("no pet state")
	}
	foe := &NPC{ServerID: 7001, Name: "Test Crab", Level: 30, HP: 3000, MaxHP: 3000, X: pet.X + 2, Z: pet.Z}
	r.w.NPCs[foe.ServerID] = foe
	bane := wbSpell(9501, 30, 1, "Test Bane", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":60,"stack":0,"level":30,
		"effects":[{"id":"0001","type":"Drain","kind":"drain","stat":"str","amount":-30,"scale":"str","scale_pct":10},
			{"id":"0001","type":"Drain","kind":"drain","stat":"ac","amount":-90}]}`)
	r.w.mu.Lock()
	r.w.petDrainStatsLocked(ps, foe, &bane, r.at(20*time.Second))
	r.w.petDrainStatsLocked(ps, foe, &bane, r.at(30*time.Second)) // again: replaces the first
	r.w.mu.Unlock()
	if m := foe.creatureMods(); m.Str != -30 || m.AC != -90 || len(foe.Effects) != 1 {
		t.Errorf("the creature: STR %+d, AC %+d, %d effects; want -30, -90, one", m.Str, m.AC, len(foe.Effects))
	}
	if m := pet.effectMods(); m.Str != 30 || m.AC != 90 {
		t.Errorf("the pet: STR %+d, AC %+d; want +30, +90 (a pet of no workbook row has no STR to scale)", m.Str, m.AC)
	}
	if ps.ownsTap() {
		t.Errorf("a pet of no template owns a tap")
	}
	ps.tmpl = &petTemplate{tmpl: &ClassTemplate{abilities: []templateAbility{{TemplateAbility: database.TemplateAbility{Kind: database.AbilityLifetap}}}}}
	if !ps.ownsTap() {
		t.Errorf("a pet whose template has a life tap does not own one")
	}
}

// A pet's area nuke lands round its target too (the user, 2026-11-01: "fix
// them all", of the workbook's Air pets' area nukes, cast on the target
// alone until then): the target takes the spell's whole figure (its hp and
// hp2, the workbook's legend: the target a second time), a creature 8 off
// it the area's own (hp2), one 40 off nothing. And a pet's root (the
// Djinni Vizier's Air Root) holds its target and the creature beside it.
func TestAPetsAreaNukeAndRootLandRoundItsTarget(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	ps := r.w.pets[r.p.ServerID]
	mk := func(id EntityID, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: "Test Crab", Level: 40, HP: 5000, MaxHP: 5000, X: pet.X + 3 + dx, Y: pet.Y, Z: pet.Z, World: pet.World}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	target, near, far := mk(7101, 0), mk(7102, 8), mk(7103, 40)
	gust := wbSpell(9601, 40, 1, "Test Gust", `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":40,"radius":15,
		"effects":[{"id":"0001","type":"Dmg/Heal","kind":"hp","amount":-100},{"id":"0001","type":"Dmg/Heal","kind":"hp2","amount":-40}]}`)
	a := &templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityNuke}}
	r.w.mu.Lock()
	r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: &gust, target: target.ServerID}, r.at(20*time.Second))
	r.w.mu.Unlock()
	tookT, tookN, tookF := 5000-target.HP, 5000-near.HP, 5000-far.HP
	if tookT <= 0 || tookN <= 0 || tookF != 0 || tookT <= tookN {
		t.Fatalf("hit: the target %d, 8 off %d, 40 off %d; want the target most, 8 off some, 40 off none", tookT, tookN, tookF)
	}

	defer func(f func(int) int) { rootIntn = f }(rootIntn)
	rootIntn = func(int) int { return 9999 } // no resist
	hold := wbSpell(9602, 40, 1, "Test Hold", `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"duration_s":10,"stack":0,"level":40,"radius":15,
		"effects":[{"id":"0002","type":"Statistic Absolute","kind":"speed","amount":-50,"absolute":true}]}`)
	root := &templateAbility{TemplateAbility: database.TemplateAbility{Kind: database.AbilityRoot}}
	r.w.mu.Lock()
	r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: root, spell: &hold, target: target.ServerID}, r.at(30*time.Second))
	r.w.mu.Unlock()
	now := r.at(31 * time.Second)
	if !target.rooted(now) || !near.rooted(now) || far.rooted(now) {
		t.Errorf("rooted: the target %v, 8 off %v, 40 off %v; want the first two", target.rooted(now), near.rooted(now), far.rooted(now))
	}
	if target.rooted(r.at(41 * time.Second)) {
		t.Errorf("the root outlasted its 10 s")
	}
}
