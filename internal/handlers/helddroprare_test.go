package handlers

import (
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A creature's held drop skips another named creature's own loot, a rare
// loot pool's item and a boss's loot (table heldbarred): the owner,
// 2026-11-19, "[a player] killed a soluseks eye fire goblin and waraxe of
// noroxxus dropped ... those ultra rare items should drop less, but noroxxus
// is only off noroxxus". It falls to the next record of the look below; a named
// creature still drops its own.
func TestHeldDropsSkipRareAndOthersNamedLoot(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "heldrare.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	model := uint32(0xC0000002)
	look := int64(int32(model))
	for _, r := range []struct {
		id    int
		name  string
		level int
	}{
		{9001, "plain axe", 10},
		{9002, "Waraxe of Noroxus", 30}, // Noroxus's own (npcloot)
		{9003, "rare axe", 40},          // a rare pool's
		{9004, "uncommon axe", 45},      // an uncommon pool's: still a held drop
		{9005, "Staff of the Boss", 50}, // a boss's (heldbarred)
		{9006, "pooled named axe", 55},  // a lootpool row naming Grog
	} {
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4,
			attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,
			itemname, itemdesc, model, color) VALUES (?, 10, 0, 1, 0, 14, 0, 0, 1, 0, 1, 5, 0, ?, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, '', ?, 0)`,
			r.id, r.level, r.name, look); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)`,
		`CREATE TABLE IF NOT EXISTS npcloot (npc_name TEXT, tablename TEXT, chance REAL, rolls INTEGER, note TEXT, source TEXT)`,
		`CREATE TABLE IF NOT EXISTS lootpool (id INTEGER PRIMARY KEY, category TEXT, region TEXT, npc_name TEXT, patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)`,
		`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES ('noroxus own', 9002, 1, 1, 100, 't')`,
		`INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES ('Noroxus', 'noroxus own', 30, 1, '', 't')`,
		`INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) VALUES ('rare', 'any', '', 9003, 1, 1, 100, 't')`,
		`INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) VALUES ('uncommon', 'any', '', 9004, 1, 1, 100, 't')`,
		`INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) VALUES ('rare', 'any', 'King Grog', 9006, 1, 1, 100, 't')`,
		`INSERT INTO heldbarred (patternid, note, source) VALUES (9005, 'a boss''s', 't')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	d, err := database.NewInventoryStore(db).HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	if d.Left["rare pool"] != 1 || d.Left["boss loot"] != 1 {
		t.Errorf("left out %v, want the rare pool's and the boss's", d.Left)
	}
	d.LevelBelow, d.LevelAbove = 100, 0 // the window off: the nearest at or below, as before it
	name := func(level int, who string) string {
		e, ok := d.Pick(uint32(0xC0000002), false, level, who, false)
		if !ok {
			return ""
		}
		return e.Pattern.ItemName
	}
	for _, c := range []struct {
		level int
		who   string
		want  string
	}{
		{35, "fire goblin", "plain axe"},      // past Noroxus's own to the record below
		{35, "Noroxus", "Waraxe of Noroxus"},  // Noroxus drops his own
		{35, "noroxus", "Waraxe of Noroxus"},  // by NPCClassKey
		{44, "fire goblin", "plain axe"},      // the rare pool's L40 is skipped
		{60, "fire goblin", "uncommon axe"},   // the boss's L50 and Grog's L55 skipped: the uncommon L45
		{60, "King Grog", "pooled named axe"}, // Grog drops the pool row naming him
		{5, "fire goblin", ""},                // none at or below: nothing
	} {
		if got := name(c.level, c.who); got != c.want {
			t.Errorf("%s at %d: %q, want %q", c.who, c.level, got, c.want)
		}
	}
}
