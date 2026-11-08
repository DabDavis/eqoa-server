package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// hitsOver is every hit npcMeleeOn lands of the NPC on the player, one for
// each face of its roll's dice (0 to its width): the least, the most, the
// mean and how many figures differ.
func hitsOver(npc *NPC, p *Player) (lo, hi int, mean float64, distinct int) {
	r := npcMeleeRollFor(npc, p)
	seen := map[int]bool{}
	lo, sum := 1<<30, 0
	for k := 0; k <= r.Width; k++ {
		h := npcMeleeOn(npc, p, func(int) int { return k })
		lo, hi, sum = min(lo, h), max(hi, h), sum+h
		seen[h] = true
	}
	return lo, hi, float64(sum) / float64(r.Width+1), len(seen)
}

// Petition 1289 (jibrish, 2026-11-08): past about 2k AC Bertoxxulous hit for
// its floor times its template's 1380% every time, and 3k AC changed nothing.
// A creature whose template boosts its melee now lands a roll under its floor
// between the floor and half of it, so its hits keep a window and more AC
// takes off ever less; where its roll is over the floor nothing changed. The
// controls: the same creature unboosted, and a boosted training dummy, still
// hit Sandstorm's floor every time.
func TestABoostedCreatureHitsInAWindowPastItsFloor(t *testing.T) {
	boss := &NPC{Level: 66, tmpl: &ClassTemplate{ClassTemplate: database.ClassTemplate{MeleePct: 1380}}}
	tank := func(ac int) *Player { return &Player{Level: 60, Class: 0, AC: ac, Agi: 400} }
	floor := npcMeleeRoll(boss).Floor
	floorHit := floor * 1380 / 100

	lo2, hi2, mean2, n2 := hitsOver(boss, tank(2000))
	lo3, hi3, mean3, n3 := hitsOver(boss, tank(3000))
	t.Logf("floor %d (boosted %d); AC 2000: %d-%d, mean %.0f, %d figures; AC 3000: %d-%d, mean %.0f, %d figures",
		floor, floorHit, lo2, hi2, mean2, n2, lo3, hi3, mean3, n3)
	if n2 < 20 || n3 < 10 {
		t.Errorf("not a window: %d figures at AC 2000, %d at AC 3000", n2, n3)
	}
	if !(mean3 < mean2 && mean2 < float64(floorHit)) {
		t.Errorf("AC past the floor counts for nothing: mean %.0f at 2000, %.0f at 3000, the floor's %d", mean2, mean3, floorHit)
	}
	if mean2-mean3 > float64(floorHit)/4 {
		t.Errorf("no diminishing return: 2000 to 3000 AC takes %.0f off a %d floor", mean2-mean3, floorHit)
	}
	if hi2 >= floorHit || lo3 < floorHit/2 {
		t.Errorf("out of the floor's band: %d at the most (floor %d), %d at the least (half %d)", hi2, floorHit, lo3, floorHit/2)
	}

	// over the floor, Sandstorm's roll as before
	p := tank(0)
	p.Agi = 0
	r := npcMeleeRollFor(boss, p)
	for k := 0; k <= r.Width; k++ {
		dice := func(int) int { return k }
		if got, want := npcMeleeOn(boss, p, dice), boss.meleeOut(r.hitOn(p, dice)); got != want {
			t.Fatalf("a roll over the floor, face %d: %d, before %d", k, got, want)
		}
	}

	// the controls: unboosted, and a dummy, the floor every time
	for name, npc := range map[string]*NPC{
		"unboosted": {Level: 66},
		"a dummy":   {Level: 66, Dummy: TrainingDummy, tmpl: boss.tmpl},
	} {
		want := npc.meleeOut(floor)
		for _, ac := range []int{2000, 3000} {
			if lo, hi, _, _ := hitsOver(npc, tank(ac)); lo != want || hi != want {
				t.Errorf("%s at AC %d: %d-%d, the floor's %d every time", name, ac, lo, hi, want)
			}
		}
	}
}
