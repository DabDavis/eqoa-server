package entity

import (
	"testing"
	"time"
)

// Rakish's pierce debuff raises a one-handed piercer's hits only, in either
// hand; another pierce debuff still raises every piercing weapon's (the
// owner, 2026-11-14).
func TestRakishOnlyOneHandedPierce(t *testing.T) {
	now := time.Now()
	rak := &ActiveEffect{Name: "Rakish Assault", Until: now.Add(time.Minute)}
	rak.Mods.Pierce = -25
	npc := &NPC{Effects: []*ActiveEffect{rak}}
	if got := npcMeleeDebuffPct(npc, StatPierce, 5, now); got != 25 {
		t.Fatalf("1HP: %d", got)
	}
	for _, at := range []byte{6, 7, 9, 10} {
		if got := npcMeleeDebuffPct(npc, StatPierce, at, now); got != 0 {
			t.Fatalf("attack type %d: %d", at, got)
		}
	}
	if got := npcMeleeDebuffPct(npc, StatPierce, 0, now); got != 25 {
		t.Fatalf("an ability's own pierce hit: %d", got)
	}
	other := &ActiveEffect{Name: "Piercing Curse", Until: now.Add(time.Minute)}
	other.Mods.Pierce = -10
	npc2 := &NPC{Effects: []*ActiveEffect{other}}
	if got := npcMeleeDebuffPct(npc2, StatPierce, 6, now); got != 10 {
		t.Fatalf("another debuff on a 2HP: %d", got)
	}
}
