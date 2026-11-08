package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Equipment a corpse rolls is at most lootLevelReach above its opener's level
// (the user, 2026-11-07: "equipment drops from mobs for players should be
// within the players level range, with in 10 levels higher/lower") and of any
// level below it (jib, 2026-11-08: "level 60 players should absolutely be able
// to farm the gear", Cazic-Thule's Rubicite armour of levels 47-48); what is
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
		{"eleven above", gear(12, 41), 30, false},
		{"level 60 armour for a level 49", gear(5, 60), 49, false},
		{"gloves, the last gear slot", gear(19, 55), 30, false},
		{"ten below", gear(12, 20), 30, true},
		{"eleven below", gear(5, 19), 30, true},
		{"a level 1 ring for a level 60", gear(8, 1), 60, true},
		{"Rubicite Boots (47) for a level 60", gear(11, 47), 60, true},
		{"Rubicite Greaves (48) for a level 59", gear(10, 48), 59, true},
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
	if cut != 1 || len(kept) != 3 || kept[0].Item.Pattern.LevelReq != 5 || kept[1].Item.Pattern.LevelReq != 30 ||
		kept[2].Item.Pattern.EquipSlot != -1 {
		t.Errorf("a level 30 opener's choices: %d kept (%+v), %d cut; want the level 5 and 30 pieces and the potion, 1 cut", len(kept), kept, cut)
	}
}

// A corpse rolled for its opener: a level 60 keeps its tables' level 1 sword,
// a level 30 loses it when the sword is level 45 and the roll lands on what is
// left; the control, the same corpse for an opener of no known level, holds
// the sword.
func TestACorpseRollsGearBelowItsOpenerButNoneFarAbove(t *testing.T) {
	open := func(swordLevel, level int) int64 {
		f := newCorpseFixture(t, 1) // an npcitem row: the Rusty Sword
		if _, err := f.db.Exec(`UPDATE itempattern SET levelreq = ? WHERE patternid = ?`, swordLevel, f.sword); err != nil {
			t.Fatal(err)
		}
		f.npc.Level = 12
		f.player.Level = level
		bindLoot(t, f, "beetle", 100, 2) // the sword or the bait, by weight
		f.h.lootIntn = func(n int) int { return 0 }
		f.h.OpenCorpse(f.sess, f.npc)
		got := f.sent()
		return lootCount(t, got[len(got)-1])
	}
	if n := open(1, 0); n != 3 {
		t.Fatalf("the control: %d records, want the npcitem sword and two rolled", n)
	}
	// level 60: a level 1 sword is kept, on the corpse and in both rolls
	if n := open(1, 60); n != 3 {
		t.Fatalf("a level 60 opener, a level 1 sword: %d records, want the npcitem sword and two rolled", n)
	}
	// level 30: a level 45 sword is gone, both rolls land on the bait (not gear)
	if n := open(45, 30); n != 2 {
		t.Fatalf("a level 30 opener, a level 45 sword: %d records, want the two rolls' bait alone", n)
	}
}
