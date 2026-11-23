package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Equipment a corpse rolls is at most lootLevelReach above its opener's level
// (the user, 2026-11-07: "equipment drops from mobs for players should be
// within the players level range, with in 10 levels higher/lower") and of any
// level below it (a GM, 2026-11-08: "level 60 players should absolutely be able
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

// A corpse is judged against its creature's level, not its opener's (loot
// determined at the spawn: the owner, 2026-11-18, ticket 2366): a level 12
// creature's level 20 sword is on its corpse whoever opens it, level 1 or 60; a
// level 45 sword is past its reach (12 + lootLevelReach) for every opener; the
// control, a level 1 sword, always.
func TestACorpseIsJudgedByItsCreaturesLevelNotItsOpeners(t *testing.T) {
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
	for _, level := range []int{1, 30, 60} {
		if n := open(1, level); n != 3 {
			t.Fatalf("a level %d opener, a level 1 sword: %d records, want the npcitem sword and two rolled", level, n)
		}
		if n := open(20, level); n != 3 {
			t.Fatalf("a level %d opener, a level 20 sword on a level 12: %d records, want the sword and two rolled", level, n)
		}
		if n := open(45, level); n != 2 {
			t.Fatalf("a level %d opener, a level 45 sword on a level 12: %d records, want the two rolls' bait alone", level, n)
		}
	}
}

// A class's spell scroll a corpse rolls is at most lootScrollReach below its
// creature's level, 60 at most (the owner, 2026-11-22, petition 2677: a level 59
// Jal'Raeth Patriarch's "level 13 rogue spell", "13 spells shouldnt drop at all");
// above it, with no level, or usable by every class (the Corroded Giant's
// illusions of levels 10 to 60) it is not judged, and something else in the
// scroll's art worn nowhere is not a scroll of a class.
func TestLootFitsKeepsLowSpellScrollsOffHighCreatures(t *testing.T) {
	scroll := func(level, classes int) *database.ItemPatternRecord {
		return &database.ItemPatternRecord{ItemIcon: scrollIcon, EquipSlot: -1, LevelReq: level, ClassUse: classes}
	}
	const rogue, every = 1 << 6, 0x7FFF
	for _, c := range []struct {
		why   string
		p     *database.ItemPatternRecord
		level int
		want  bool
	}{
		{"a level 13 rogue scroll off a level 59", scroll(13, rogue), 59, false},
		{"a level 48 scroll off a level 59", scroll(48, rogue), 59, false},
		{"a level 49 scroll off a level 59", scroll(49, rogue), 59, true},
		{"a level 59 scroll off a level 59", scroll(59, rogue), 59, true},
		{"a level 60 scroll off a level 40", scroll(60, rogue), 40, true},
		{"a level 50 scroll off a level 70 boss", scroll(50, rogue), 70, true},
		{"a level 49 scroll off a level 70 boss", scroll(49, rogue), 70, false},
		{"a level 10 scroll off a level 20", scroll(10, rogue), 20, true},
		{"a level 9 scroll off a level 20", scroll(9, rogue), 20, false},
		{"a level 1 scroll off a level 5", scroll(1, rogue), 5, true},
		{"three classes' scroll", scroll(13, 1<<7|1<<11|1<<13), 59, false},
		{"an illusion every class may use", scroll(10, every), 60, true},
		{"a scroll's art with no class mask", scroll(13, 0), 60, true},
		{"a scroll with no level", scroll(0, rogue), 60, true},
		{"no creature level known", scroll(13, rogue), 0, true},
		{"not the scroll's art", &database.ItemPatternRecord{ItemIcon: 1, EquipSlot: -1, LevelReq: 13, ClassUse: rogue}, 59, true},
	} {
		if got := lootFits(c.p, c.level); got != c.want {
			t.Errorf("%s: %v, want %v", c.why, got, c.want)
		}
	}
	item := func(p *database.ItemPatternRecord) database.LootChoice {
		return database.LootChoice{Weight: 1, Item: database.CharInventoryEntry{Pattern: *p}}
	}
	kept, cut := fitChoices([]database.LootChoice{item(scroll(13, rogue)), item(scroll(55, rogue)), item(scroll(20, every))}, 59)
	if cut != 1 || len(kept) != 2 || kept[0].Item.Pattern.LevelReq != 55 || kept[1].Item.Pattern.LevelReq != 20 {
		t.Errorf("a level 59 creature's scrolls: %d kept (%+v), %d cut; want the level 55 and the every-class 20, 1 cut", len(kept), kept, cut)
	}
}
