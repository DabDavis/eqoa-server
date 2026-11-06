package entity

import (
	"testing"
	"time"
)

// A player's effects window shows its pet's effects after its own (petition
// 1041: a necromancer's pet buffs were on the pet alone), each named one not
// run out, at most MaxEffects in all; with no pet, its own alone.
func TestThePetsBuffsShowAfterTheOwners(t *testing.T) {
	w := NewWorld()
	now := time.Now()
	p := &Player{ServerID: 7, Buffs: []BuffSlot{{Icon: 1, Name: "Dark Pact"}}}
	if got := w.buffSlotsLocked(p, now); len(got) != 1 {
		t.Fatalf("no pet: %v", got)
	}
	pet := &NPC{ServerID: 900, Effects: []*ActiveEffect{
		{Name: "Soul Mastery", Icon: 2, Until: now.Add(time.Minute)},
		{Name: "", Icon: 3, Until: now.Add(time.Minute)},                   // no name: no slot
		{Name: "Mastery of Plague", Icon: 4, Until: now.Add(-time.Second)}, // run out
	}}
	w.pets = map[EntityID]*petState{p.ServerID: {owner: p, npc: pet}}
	got := w.buffSlotsLocked(p, now)
	if len(got) != 2 || got[0].Name != "Dark Pact" || got[1].Name != "Soul Mastery" || got[1].Icon != 2 {
		t.Fatalf("with a pet: %v", got)
	}
	if len(p.Buffs) != 1 {
		t.Fatalf("the player's own slots changed: %v", p.Buffs)
	}
	for len(p.Buffs) < MaxEffects-1 {
		p.Buffs = append(p.Buffs, BuffSlot{Icon: 9, Name: "x"})
	}
	pet.Effects = append(pet.Effects, &ActiveEffect{Name: "Soul Mastery +", Icon: 5, Until: now.Add(time.Minute)})
	if got := w.buffSlotsLocked(p, now); len(got) != MaxEffects || got[MaxEffects-1].Name != "Soul Mastery" {
		t.Fatalf("the cap: %d slots, last %v", len(got), got[len(got)-1])
	}
	pet.Dead = true
	if got := w.buffSlotsLocked(p, now); len(got) != MaxEffects-1 {
		t.Fatalf("a dead pet's: %d slots", len(got))
	}
}
