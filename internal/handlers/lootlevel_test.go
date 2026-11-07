package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Equipment a corpse rolls is within lootLevelReach of its opener's level
// (the user, 2026-11-07: "equipment drops from mobs for players should be
// within the players level range, with in 10 levels higher/lower"); what is
// not gear, an item with no level and a corpse with no known opener are not
// judged.
func TestLootFitsTheOpenersLevels(t *testing.T) {
	gear := func(slot, level int) *database.ItemPatternRecord {
		return &database.ItemPatternRecord{EquipSlot: slot, LevelReq: level}
	}
	for _, c := range []struct {
		why   string
		p     *database.ItemPatternRecord
		level int
		want  bool
	}{
		{"its own level", gear(12, 30), 30, true},
		{"ten above", gear(12, 40), 30, true},
		{"ten below", gear(12, 20), 30, true},
		{"eleven above", gear(12, 41), 30, false},
		{"eleven below", gear(5, 19), 30, false},
		{"a level 1 ring for a level 60", gear(8, 1), 60, false},
		{"gloves, the last gear slot", gear(19, 55), 30, false},
		{"no opener known", gear(12, 55), 0, true},
		{"no level of its own", gear(12, 0), 60, true},
		{"not worn", &database.ItemPatternRecord{EquipSlot: -1, LevelReq: 55}, 10, true},
		{"a thrown potion", &database.ItemPatternRecord{EquipSlot: 16, AttackType: 10, LevelReq: 60}, 10, true},
		{"a fishing pole", gear(20, 55), 10, true},
		{"a tradeskill tool", gear(23, 55), 10, true},
	} {
		if got := lootFits(c.p, c.level); got != c.want {
			t.Errorf("%s (slot %d, level %d, opener %d): %v, want %v", c.why, c.p.EquipSlot, c.p.LevelReq, c.level, got, c.want)
		}
	}
	item := func(slot, level int) database.LootChoice {
		return database.LootChoice{Weight: 1, Item: database.CharInventoryEntry{Pattern: *gear(slot, level)}}
	}
	kept, cut := fitChoices([]database.LootChoice{item(12, 5), item(12, 30), item(12, 55), item(-1, 55)}, 30)
	if cut != 2 || len(kept) != 2 || kept[0].Item.Pattern.LevelReq != 30 || kept[1].Item.Pattern.EquipSlot != -1 {
		t.Errorf("a level 30 opener's choices: %d kept (%+v), %d cut; want the level 30 piece and the potion, 2 cut", len(kept), kept, cut)
	}
}

// A corpse rolled for a level 60 opener: its tables' level 1 sword is left out
// and the roll lands on what is left; the control, the same corpse for an
// opener of no known level, holds the sword as before.
func TestACorpseRollsNoGearFarFromItsOpener(t *testing.T) {
	open := func(level int) int64 {
		f := newCorpseFixture(t, 1) // an npcitem row: a level 1 Rusty Sword
		f.npc.Level = 12
		f.player.Level = level
		bindLoot(t, f, "beetle", 100, 2) // the sword or the bait, by weight
		f.h.lootIntn = func(n int) int { return 0 }
		f.h.OpenCorpse(f.sess, f.npc)
		got := f.sent()
		return lootCount(t, got[len(got)-1])
	}
	if n := open(0); n != 3 {
		t.Fatalf("the control: %d records, want the npcitem sword and two rolled", n)
	}
	// level 60: the npcitem sword gone, both rolls land on the bait (not gear)
	if n := open(60); n != 2 {
		t.Fatalf("a level 60 opener: %d records, want the two rolls' bait alone", n)
	}
}
