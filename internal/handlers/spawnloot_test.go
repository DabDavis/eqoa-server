package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// Loot is determined at the spawn (the owner, 2026-11-18, ticket 2366; entity/
// spawnloot.go): a corpse's dice are its spawn's seed, so the same spawn's corpse
// holds the same items and coins whenever it dies and whoever opens it; another
// spawn (another seed) rolls anew.
func TestACorpsesLootIsItsSpawnsWhoeverOpensIt(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.npc.Level = 15
	bindLoot(t, f, "beetle", 50, 12) // a level 15 rolls the sword or the bait, half the time, 12 times
	f.h.lootIntn, f.h.lootFloat, f.h.coinRoll = nil, nil, nil
	seed := uint64(0xC0FFEE)
	f.h.lootSeed = func(*entity.NPC) uint64 { return seed }
	roll := func(died time.Time) string {
		c := f.h.rollCorpse(f.npc, died, f.npc.Level)
		s := fmt.Sprintf("%d tunar:", c.tunar)
		for _, it := range c.items {
			s += " " + it.Pattern.ItemName
		}
		return s
	}
	first := roll(time.Now())
	for i := 0; i < 5; i++ {
		if again := roll(time.Now().Add(time.Duration(i) * time.Hour)); again != first {
			t.Fatalf("the same spawn rolled %q, then %q", first, again)
		}
	}
	differs := false
	for s := uint64(1); s <= 8 && !differs; s++ {
		seed = s
		differs = roll(time.Now()) != first
	}
	if !differs {
		t.Errorf("eight other spawns all rolled %q", first)
	}
	// a test's own dice still win over the seed
	seed = 0xC0FFEE
	f.h.lootIntn = func(n int) int { return 0 }
	f.h.lootFloat = func() float64 { return 0 }
	f.h.coinRoll = func(int) int { return 0 }
	if c := f.h.rollCorpse(f.npc, time.Now(), f.npc.Level); c.tunar != 1 {
		t.Errorf("the hooks' coins: %d, want 1 (1 + the roll's 0: corpsecoins.go)", c.tunar)
	}
}
