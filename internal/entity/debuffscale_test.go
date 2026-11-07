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
	if got := npcSpellResistPct(60, npc, StatAR, now); got != 35 {
		t.Errorf("a level 70's resist of a level 60's arcane spell: %d%%, want 35", got)
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
