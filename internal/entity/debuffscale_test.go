package entity

import (
	"fmt"
	"testing"
	"time"
)

// A hit with a debuff_scale grows with what its target's resist of that kind
// is lowered by (petition 1036: Agony's three hits, about 550 each from a
// rogue of 550 DEX, near 800 after Rakish Assault's 100 off pierce): the
// largest debuff of the kind counts, another kind's or none leaves the hit as
// it is, and an effect without the field never grows.
func TestAHitGrowsWithItsTargetsResistDebuff(t *testing.T) {
	d, err := ParseSpellJSON("Agony", "", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"effects":[
		{"id":"064F","type":"Dmg/Heal","kind":"hp","amount":-1,"scale":"dex","scale_pct":100,"resist":"pr","debuff_scale":{"stat":"pierce","pct":0.45}},
		{"id":"0650","type":"Dmg/Heal","kind":"hp","amount":-1,"scale":"dex","scale_pct":100,"resist":"pr"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Effects) != 2 || d.Effects[0].DebuffStat != StatPierce || d.Effects[0].DebuffPct != 0.45 || d.Effects[1].DebuffPct != 0 {
		t.Fatalf("parsed %+v", d.Effects)
	}
	now := time.Now()
	npc := &NPC{}
	if got := debuffScaled(d.Effects[0], npc, -551, now); got != -551 {
		t.Errorf("no debuff: %d, want -551", got)
	}
	rakish := &ActiveEffect{Until: now.Add(time.Minute)}
	rakish.Mods.Pierce = -100
	misery := &ActiveEffect{Until: now.Add(time.Minute)}
	misery.Mods.Pierce = -60
	npc.Effects = []*ActiveEffect{misery, rakish}
	if got := debuffScaled(d.Effects[0], npc, -551, now); got != -799 {
		t.Errorf("after Rakish: %d, want -799 (551 x 1.45)", got)
	}
	if got := debuffScaled(d.Effects[1], npc, -551, now); got != -551 {
		t.Errorf("an effect without the field: %d", got)
	}
	gone := &NPC{Effects: []*ActiveEffect{{Until: now.Add(-time.Second), Mods: rakish.Mods}}}
	if got := debuffScaled(d.Effects[0], gone, -551, now); got != -551 {
		t.Errorf("a debuff run out: %d", got)
	}
	fr := &ActiveEffect{Until: now.Add(time.Minute)}
	fr.Mods.FR = -100
	if got := debuffScaled(d.Effects[0], &NPC{Effects: []*ActiveEffect{fr}}, -551, now); got != -551 {
		t.Errorf("another kind's debuff: %d", got)
	}
}

// An arcane resist debuff on a creature (petition 1176, Jibrish) hits harder with arcane spells alone: one percent
// more for each arcaneDebuffPer points of the strongest one on, and it takes the resist off besides.
func TestAnArcaneDebuffRaisesArcaneDamage(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	npc := &NPC{ServerID: 9, Level: 70}
	if got := arcaneDebuffed(StatAR, npc, -1000, now); got != -1000 {
		t.Errorf("no debuff on: %d", got)
	}
	if got := arcaneDebuffed(StatAR, nil, -1000, now); got != -1000 {
		t.Errorf("no creature: %d", got)
	}
	if got := npcSpellResistPct(60, npc, StatAR, now); got != 15 {
		t.Errorf("a level 70's resist of a level 60's arcane spell: %d%%, want 15", got)
	}
	npc.Effects = []*ActiveEffect{
		{SpellID: 1, Until: now.Add(time.Minute), Mods: StatMods{AR: -155}},  // Paralytic Punishment at 300 INT
		{SpellID: 2, Until: now.Add(time.Minute), Mods: StatMods{AR: -25}},   // a lower one does not add
		{SpellID: 3, Until: now.Add(-time.Second), Mods: StatMods{AR: -900}}, // ended
	}
	if got := arcaneDebuffed(StatAR, npc, -1000, now); got != -1100 {
		t.Errorf("155 off: %d, want 10%% more", got)
	}
	if got := arcaneDebuffed(StatFR, npc, -1000, now); got != -1000 {
		t.Errorf("a fire spell: %d", got)
	}
	if got := arcaneDebuffed(StatNone, npc, -1000, now); got != -1000 {
		t.Errorf("a spell of no resist: %d", got)
	}
	if got := arcaneDebuffed(StatAR, npc, 500, now); got != 500 {
		t.Errorf("not a hit: %d", got)
	}
	if got := npcSpellResistPct(60, npc, StatAR, now); got != 0 {
		t.Errorf("its resist with the debuff on: %d%%, want none", got)
	}
	npc.Effects[0].Mods.AR = -14
	if got := arcaneDebuffed(StatAR, npc, -1000, now); got != -1010 {
		t.Errorf("25 off, the stronger now: %d, want 1%% more", got)
	}
}

// The same through a landing: an arcane spell's hit and tick on a creature come to a tenth more with the debuff on,
// a fire spell's do not.
func TestAnArcaneDebuffLandsHarder(t *testing.T) {
	r, _, _ := charmRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = func(int) int { return 99 } // nothing resisted
	crab := r.enemy
	crab.HP, crab.MaxHP = 5000000, 5000000
	spell := func(id int, kind, resist string, duration int) CastSpell {
		return wbSpell(id, 20, 1, "Test "+kind, fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":%d,"stack":0,"level":20,"min_level":1,"req_equipment":"Any",
			"effects":[{"id":"0001","type":"Dmg/Heal","kind":"%s","amount":-10000,"resist":"%s"}]}`, duration, kind, resist))
	}
	took := func(s CastSpell, ticks bool) int {
		before := crab.HP
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
		if ticks {
			for _, e := range crab.Effects {
				if e.SpellID == s.SpellID {
					e.NextTick = r.at(2 * time.Second)
					r.w.tickNPCEffectLocked(crab, e, r.at(2*time.Second))
					e.Until = r.at(0) // off the debuff's count, and no more ticks
				}
			}
		}
		return before - crab.HP
	}
	bolt, flame, rot := spell(9301, "hp", "ar", 0), spell(9302, "hp", "fr", 0), spell(9303, "hp_tick", "ar", 60)
	boltBefore, flameBefore, rotBefore := took(bolt, false), took(flame, false), took(rot, true)
	if boltBefore <= 0 || flameBefore <= 0 || rotBefore <= 0 {
		t.Fatalf("no damage to compare: %d, %d, %d", boltBefore, flameBefore, rotBefore)
	}
	crab.Effects = append(crab.Effects, &ActiveEffect{SpellID: 1, Until: r.at(time.Hour), Mods: StatMods{AR: -155}})
	near := func(got, want int) bool { return got >= want-2 && got <= want+2 }
	if got := took(bolt, false); !near(got, boltBefore*110/100) {
		t.Errorf("an arcane hit with the debuff on: %d, want 10%% over %d", got, boltBefore)
	}
	if got := took(flame, false); got != flameBefore {
		t.Errorf("a fire hit with the debuff on: %d, want %d", got, flameBefore)
	}
	if got := took(rot, true); !near(got, rotBefore*110/100) {
		t.Errorf("an arcane tick with the debuff on: %d, want 10%% over %d", got, rotBefore)
	}
}

// A template's resist_kind (petition 1178, Jibrish): spell damage of that kind comes to what resist_pct leaves, through
// a hit, a tick, a proc and a damage shield; another kind's and a creature of no template's come whole; a debuff of the
// kind takes some off, and something always lands.
func TestACreatureStrongInAResist(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &ClassTemplate{}
	tm.ResistKind, tm.ResistPct = "FR", 75
	npc := &NPC{ServerID: 9, Level: 54, tmpl: tm}
	if got := creatureResists(StatFR, npc, -1000, now); got != -250 {
		t.Errorf("fire at 75%%: %d, want -250", got)
	}
	if got := creatureResists(StatCR, npc, -1000, now); got != -1000 {
		t.Errorf("cold: %d", got)
	}
	if got := creatureResists(StatNone, npc, -1000, now); got != -1000 {
		t.Errorf("no resist: %d", got)
	}
	if got := creatureResists(StatFR, &NPC{}, -1000, now); got != -1000 {
		t.Errorf("no template: %d", got)
	}
	if got := creatureResists(StatFR, nil, -1000, now); got != -1000 {
		t.Errorf("no creature: %d", got)
	}
	if got := creatureResists(StatFR, npc, 500, now); got != 500 {
		t.Errorf("not a hit: %d", got)
	}
	npc.Effects = []*ActiveEffect{{SpellID: 1, Until: now.Add(time.Minute), Mods: StatMods{FR: -105}}}
	if got := creatureResists(StatFR, npc, -1000, now); got != -500 {
		t.Errorf("with 105 FR off it, 25 points of the 75: %d, want -500", got)
	}
	npc.Effects = nil
	tm.ResistPct = 100
	if got := creatureResists(StatFR, npc, -1000, now); got != 0 {
		t.Errorf("at 100%%: %d, want nothing (elemental.go)", got)
	}
	func() {
		defer withResistModel(ResistModel{Version: ResistModelLegacy})()
		if got := creatureResists(StatFR, npc, -1000, now); got != -1 {
			t.Errorf("legacy, at 100%%: %d, want 1 landing", got)
		}
	}()
	tm.ResistPct = 75

	r, _, _ := charmRig(t)
	defer func(f func(int) int) { procIntn = f }(procIntn)
	procIntn = func(int) int { return 99 }
	crab := r.enemy
	crab.HP, crab.MaxHP = 5000000, 5000000
	spell := func(id int, kind, resist string, duration int) CastSpell {
		return wbSpell(id, 20, 1, "Test "+kind, fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":%d,"stack":0,"level":20,"min_level":1,"req_equipment":"Any",
			"effects":[{"id":"0001","type":"Dmg/Heal","kind":"%s","amount":-10000,"resist":"%s"}]}`, duration, kind, resist))
	}
	took := func(s CastSpell, ticks bool) int {
		before := crab.HP
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
		if ticks {
			for _, e := range crab.Effects {
				if e.SpellID == s.SpellID {
					e.NextTick = r.at(2 * time.Second)
					r.w.tickNPCEffectLocked(crab, e, r.at(2*time.Second))
					e.Until = r.at(0)
				}
			}
		}
		return before - crab.HP
	}
	flame, frost, burn := spell(9311, "hp", "fr", 0), spell(9312, "hp", "cr", 0), spell(9313, "hp_tick", "fr", 60)
	flameWhole, frostWhole, burnWhole := took(flame, false), took(frost, false), took(burn, true)
	crab.tmpl = tm
	near := func(got, want int) bool { return got >= want-2 && got <= want+2 }
	if got := took(flame, false); !near(got, flameWhole/4) {
		t.Errorf("a fire hit on it: %d, want a quarter of %d", got, flameWhole)
	}
	if got := took(frost, false); got != frostWhole {
		t.Errorf("a cold hit on it: %d, want %d", got, frostWhole)
	}
	if got := took(burn, true); !near(got, burnWhole/4) {
		t.Errorf("a fire tick on it: %d, want a quarter of %d", got, burnWhole)
	}

	// a fire proc and a fire damage shield
	proc := &ActiveEffect{SpellID: 2, Name: "Flame", Until: r.at(time.Hour), Proc: 4000, ProcMax: 4000, Rules: EffectRules{Resist: StatFR}}
	shield := &ActiveEffect{SpellID: 3, Name: "Embers", Until: r.at(time.Hour), Shield: &Shield{Min: 4000, Max: 4000}, Rules: EffectRules{Resist: StatFR}}
	r.p.Effects = []*ActiveEffect{proc, shield}
	r.p.Int = 2000 // its proc always fires
	procIntn = func(n int) int {
		if n == 100 {
			return 0
		}
		return 0
	}
	resistless := func(f func() bool) int {
		before := crab.HP
		r.w.mu.Lock()
		f()
		r.w.mu.Unlock()
		return before - crab.HP
	}
	crab.Level = r.p.Level // nothing resisted: 0 at or under the bearer's level less one
	crab.Level -= 2
	shieldTook := resistless(func() bool { return r.w.playerShieldsLocked(r.sess, r.p, crab, r.at(time.Minute)) })
	crab.tmpl = nil
	shieldWhole := resistless(func() bool { return r.w.playerShieldsLocked(r.sess, r.p, crab, r.at(time.Minute)) })
	if shieldTook <= 0 || !near(shieldTook, shieldWhole/4) {
		t.Errorf("a fire damage shield on it: %d, want a quarter of %d", shieldTook, shieldWhole)
	}
	crab.tmpl = tm
	procTook := resistless(func() bool { return r.w.playerProcsLocked(r.sess, r.p, crab, r.at(time.Minute)) })
	crab.tmpl = nil
	procWhole := resistless(func() bool { return r.w.playerProcsLocked(r.sess, r.p, crab, r.at(time.Minute)) })
	if procTook <= 0 || !near(procTook, procWhole/4) {
		t.Errorf("a fire proc on it: %d, want a quarter of %d", procTook, procWhole)
	}
}
