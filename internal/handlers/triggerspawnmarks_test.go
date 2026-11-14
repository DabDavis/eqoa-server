package handlers

import (
	"math"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A spawn's spots (the listed place nearest the run's: an Erych patroller in the quarter its
// placeholder fell), untouchable (its copy carries the NPC bit) and noloot (its corpse is of
// nothing). The control: a plain spawn's copy is neither.
func TestTriggerSpawnSpotsUntouchableNoLoot(t *testing.T) {
	w := newTrigWorld(t)
	w.set(40, "Quarters", "near", 100, `[{"do":"spawn","npc":"Orc Pawn","spread":1,"minutes":10,"untouchable":true,"noloot":true,
		"spots":[[25100,50,15000],[24990,50,15010],[24000,50,15000]]}]`)
	if _, err := w.fire(40); err != nil {
		t.Fatal(err)
	}
	s := w.spawned()
	if len(s) != 1 || math.Hypot(float64(s[0].X-24990), float64(s[0].Z-15010)) > 2 {
		t.Fatalf("the spawn stands at %+v, want the spot nearest the GM", s)
	}
	if s[0].NPCType&0x80 == 0 || !s[0].NoLoot || !s[0].CorpseOfNothing() {
		t.Fatalf("untouchable %v noloot %v", s[0].NPCType&0x80 != 0, s[0].NoLoot)
	}
	w.set(41, "Plain", "near", 100, `[{"do":"spawn","npc":"Orc Pawn","spread":1,"minutes":10,"hostile":true}]`)
	w.e.StopRuns(0)
	if _, err := w.fire(41); err != nil {
		t.Fatal(err)
	}
	if s := w.spawned(); len(s) != 1 || s[0].NoLoot || s[0].NPCType&0x80 != 0 {
		t.Fatalf("plain copy marked: %+v", s)
	}
	bad := database.GMTrigger{Slot: 1, Scope: "near", World: -1, Steps: `[{"do":"spawn","npc":"X","untouchable":true,"hostile":true}]`}
	if len(database.TriggerProblems(bad)) == 0 {
		t.Fatal("an untouchable hostile spawn passed")
	}
}
