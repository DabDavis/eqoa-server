package entity

import (
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
