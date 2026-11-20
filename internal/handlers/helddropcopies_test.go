package handlers

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A copy of a record (an import copy, itemimported; an Ascended record, itemascended) keeps its
// base's place in the held drops (database.HeldDrops: barCopies, copiesOnlyFor): the owner,
// 2026-11-20, "some mobs are dropping boss loot still" (the import copy of the Sky constructs'
// staff, its base a rare pool's, off ordinary level 60 creatures holding a staff). A copy of a
// barred base is barred as it is, of a named creature's own loot that creature's alone, an import
// copy of an Ascended copy of a barred base too; a copy of a free base stays a held drop.
func TestHeldDropsCopiesKeepTheirBasesBar(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "heldcopies.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	staff, ripper, blade, axe := uint32(0xC0000011), uint32(0xC0000012), uint32(0xC0000013), uint32(0xC0000014)
	for _, r := range []struct {
		id    int
		name  string
		level int
		model uint32
	}{
		{9100, "plain staff", 10, staff},
		{9101, "Pneumatic Staff", 60, staff},         // a rare pool's
		{9102, "Pneumatic Staff", 60, staff},         // its import copy
		{9200, "plain ripper", 5, ripper},            //
		{9201, "Two Horned Ripper", 20, ripper},      // the Darkwood seer's own (npcloot)
		{9202, "Two Horned Ripper", 20, ripper},      // its import copy
		{9300, "plain blade", 10, blade},             //
		{9301, "Vallon's Blade", 60, blade},          // a boss's (heldbarred)
		{9302, "Ascended Vallon's Blade", 60, blade}, // its Ascended copy
		{9303, "Ascended Vallon's Blade", 60, blade}, // an import copy of the Ascended copy
		{9400, "free axe", 30, axe},                  // no bar
		{9401, "Ascended free axe", 25, axe},         // its Ascended copy, a lower level: still a held drop
	} {
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4,
			attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,
			itemname, itemdesc, model, color) VALUES (?, 10, 0, 1, 0, 14, 0, 0, 1, 0, 1, 5, 0, ?, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, '', ?, 0)`,
			r.id, r.level, r.name, int64(int32(r.model))); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS loottables (id INTEGER PRIMARY KEY, tablename TEXT, patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)`,
		`CREATE TABLE IF NOT EXISTS npcloot (npc_name TEXT, tablename TEXT, chance REAL, rolls INTEGER, note TEXT, source TEXT)`,
		`CREATE TABLE IF NOT EXISTS lootpool (id INTEGER PRIMARY KEY, category TEXT, region TEXT, npc_name TEXT, patternid INTEGER, weight INTEGER, minlevel INTEGER, maxlevel INTEGER, source TEXT)`,
		`INSERT INTO lootpool (category, region, npc_name, patternid, weight, minlevel, maxlevel, source) VALUES ('super rare', 'plane of sky', '', 9101, 1, 1, 100, 't')`,
		`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel, source) VALUES ('two horned ripper', 9201, 1, 1, 100, 't')`,
		`INSERT INTO npcloot (npc_name, tablename, chance, rolls, note, source) VALUES ('darkwood seer', 'two horned ripper', 10, 1, '', 't')`,
		`INSERT INTO heldbarred (patternid, note, source) VALUES (9301, 'a boss''s', 't')`,
		`INSERT INTO itemimported (patternid, base) VALUES (9102, 9101), (9202, 9201), (9303, 9302)`,
		`INSERT INTO itemascended (patternid, base) VALUES (9302, 9301), (9401, 9400)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	d, err := database.NewInventoryStore(db).HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	if d.Left["rare pool"] != 2 || d.Left["boss loot"] != 3 {
		t.Errorf("left out %v, want the staff and its copy (rare pool), the blade and its two copies (boss loot)", d.Left)
	}
	name := func(model uint32, level int, who string) string {
		e, ok := d.Pick(model, false, level, who)
		if !ok {
			return ""
		}
		return fmt.Sprintf("%s/%d", e.Pattern.ItemName, e.Pattern.PatternID)
	}
	for _, c := range []struct {
		model uint32
		level int
		who   string
		want  string
	}{
		{staff, 64, "agra", "plain staff/9100"},                 // never the staff's copy
		{ripper, 40, "overlord frank", "plain ripper/9200"},     // never the seer's copy
		{ripper, 40, "Darkwood seer", "Two Horned Ripper/9201"}, // the seer drops its own
		{blade, 65, "solusek champion", "plain blade/9300"},     // never the boss's copies
		{axe, 28, "orc pawn", "Ascended free axe/9401"},         // a free base's copy drops
		{axe, 40, "orc pawn", "free axe/9400"},                  //
	} {
		if got := name(c.model, c.level, c.who); got != c.want {
			t.Errorf("%s (level %d): %q, want %q", c.who, c.level, got, c.want)
		}
	}
}
