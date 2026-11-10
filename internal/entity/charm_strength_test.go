package entity

import (
	"math/rand"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// liveishBalance is the level and con tables as eqoaclassic held them on
// 2026-11-09 (balancelevel 50 and 60, balancecon 0..8 after the 11-07 cut).
func liveishBalance() *Balance {
	return NewBalance([]database.BalanceLevel{
		{Level: 50, HPPct: 340, DamagePct: 190, ConPct: 100, PetHPPct: 100, PetDamagePct: 100},
		{Level: 60, HPPct: 400, DamagePct: 160, ConPct: 100, PetHPPct: 100, PetDamagePct: 100},
	}, []database.BalanceCon{{Diff: 0, DamagePct: 100, TakenPct: 100}, {Diff: 1, DamagePct: 147, TakenPct: 100},
		{Diff: 2, DamagePct: 140, TakenPct: 100}, {Diff: 3, DamagePct: 190, TakenPct: 100}, {Diff: 4, DamagePct: 220, TakenPct: 100},
		{Diff: 5, DamagePct: 230, TakenPct: 90}, {Diff: 6, DamagePct: 230, TakenPct: 60}, {Diff: 7, DamagePct: 230, TakenPct: 50},
		{Diff: 8, DamagePct: 230, TakenPct: 45}})
}

// Petition 1353 (Fujiko): "level 52 Charging Rhino barely crit it down. Might
// be intended but Rhino was swinging for 12's against Kobolds". A charmed
// creature hits at its own strength (charmedHitLocked: as a creature hits a
// creature, the guards' npcHitNPCLocked): a level 52 on the camp's level 62
// kobolds lands 48-250, never the 12 every hit of its was on the live log (its
// floor of 26 cut to 45% by the kobolds' con on a level 52), and on its own
// level about what it lands on a player's tank. The controls: the same roll on
// a summoned pet's path is unchanged (petMeleeHit, petDealtLocked: 12 on the
// kobold), and a buff's STR on the charmed creature raises its hits.
func TestACharmedCreatureHitsAtItsOwnStrength(t *testing.T) {
	w := NewWorld()
	w.balance = liveishBalance()
	rng := rand.New(rand.NewSource(1353))
	rhino := &NPC{ServerID: 1, Name: "charging rhinoceros", Level: 52, Owner: 9, Charm: &charmHold{}}
	kobold := &NPC{ServerID: 2, Name: "kobold slayer", Level: 62}
	lo, hi, sum := 1<<30, 0, 0
	const n = 5000
	for range n {
		h := w.charmedHitLocked(rhino, kobold, rng.Intn)
		lo, hi, sum = min(lo, h), max(hi, h), sum+h
	}
	if lo != 48 || hi < 240 || hi > 260 || sum/n < 100 || sum/n > 140 {
		t.Errorf("a charmed 52 on a 62: %d-%d, %d a hit on average; want 48 (its floor of 26 at 184%%) to ~250, ~120", lo, hi, sum/n)
	}
	if today := w.petDealtLocked(rhino, kobold, petMeleeHit(rhino, kobold, rng.Intn)); today != 12 {
		t.Errorf("the control: the pet's path gives %d on the kobold, want the live log's 12", today)
	}
	buffed := *rhino
	buffed.Effects = []*ActiveEffect{{Mods: StatMods{Str: 200}}}
	bsum := 0
	for range n {
		bsum += w.charmedHitLocked(&buffed, kobold, rng.Intn)
	}
	if bsum <= sum {
		t.Errorf("the control: 200 STR on it gives %d a hit, no more than %d", bsum/n, sum/n)
	}
}
