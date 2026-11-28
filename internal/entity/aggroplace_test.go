package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Heirloom petition 3508: the name werewolf is peaceful everywhere by its
// npcaggro row (jib's video audit), and an npcaggroplace row makes the ones
// that spawned in the Unkempt Woods attack on sight, the name's radius kept.
// The controls: a werewolf that spawned outside the place stays peaceful, and
// a smaller peaceful place inside the big one wins there.
func TestAPlaceRowOverTheNamesAggroRule(t *testing.T) {
	f := newAggroFix(t, 40)
	in := f.npc(3101, "werewolf", 40, bearBody, 1010, 1000)
	out := f.npc(3102, "werewolf", 40, bearBody, 1400, 1000)
	calm := f.npc(3103, "Werewolf", 40, bearBody, 1020, 1060)
	f.w.SetAggroRules(map[string]AggroRule{"werewolf": {Aggressive: false, Radius: 45}})
	f.w.mu.Lock()
	h := f.w.hostileLocked(in)
	f.w.mu.Unlock()
	if h {
		t.Fatal("before any place row the werewolf is hostile")
	}
	f.w.SetAggroPlaces([]database.NPCAggroPlace{
		{Name: "werewolf", World: 0, X: 1000, Z: 1000, Radius: 150, Aggressive: true},
		{Name: "WEREWOLF", World: 0, X: 1020, Z: 1060, Radius: 10, Aggressive: false},
		{Name: "werewolf", World: 0, X: 1400, Z: 1000, Radius: 0, Aggressive: true}, // holds nothing
	})
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if !f.w.hostileLocked(in) {
		t.Error("the place's werewolf does not attack on sight")
	}
	if r, ok := f.w.aggroRuleLocked(in); !ok || r.Radius != 45 {
		t.Errorf("the name's radius lost: %+v %v", r, ok)
	}
	if f.w.hostileLocked(out) {
		t.Error("a werewolf outside the place attacks")
	}
	if f.w.hostileLocked(calm) {
		t.Error("the smaller peaceful place inside did not win")
	}
}
