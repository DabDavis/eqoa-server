package database

import (
	"fmt"
	"testing"
)

// creationDB is homeDB with racedefs rows and start items: a giant passing as an
// ogre with start rows of its own (in Oggok's place, but its own spot for the
// warrior), a cat folk passing as a western human with none, and two retail
// races' start items (the troll's toad snacks and sword, the human west's rations).
func creationDB(t *testing.T) (*SQLiteDB, *SQLiteCharacterStore) {
	t.Helper()
	db, s := homeDB(t)
	for _, q := range []string{
		`INSERT INTO racedefs (id, name, short_name, wire_race, body_male, body_female) VALUES (11, 'Hill Giant', 'HGT', 9, -5117396, -5117396)`,
		`INSERT INTO racedefs (id, name, short_name, wire_race, body_male, body_female, city) VALUES (12, 'Kerran', 'KER', 0, 572940888, -1219903657, 2)`,
		`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		 VALUES (0, 11, 0, 1, 0, 0, 0, 0, 9300.0, 53.2, 7300.0, 0, 1),
		        (8, 0, 2, 1, 0, 0, 0, 0, 4400.0, 58.0, 17100.0, 0, 0),
		        (1, 0, 2, 1, 0, 0, 0, 0, 9005.0, 57.0, 12749.0, 0, 0),
		        (0, 5, 0, 1, 0, 0, 0, 0, 25408.0, 70.0, 31410.0, 0, 0),
		        (8, 5, 0, 1, 0, 0, 0, 0, 25267.0, 64.0, 31425.0, 0, 0)`,
		`INSERT INTO startitems (race, class, humtype, patternid, count) VALUES
		 (5, 0, 0, 26435, 99), (5, 0, 0, 1297, 1), (5, 0, 0, 715000, 1),
		 (5, 8, 0, 5488, 1), (5, 8, 0, 26435, 99), (5, 8, 0, 715000, 1),
		 (0, 10, 1, 8398, 99), (0, 10, 1, 5489, 1), (0, 10, 1, 715000, 1),
		 (0, 12, 2, 8398, 99), (0, 12, 2, 5489, 1), (0, 12, 2, 715000, 1),
		 (8, 13, 0, 5489, 1), (7, 9, 0, 5488, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []struct {
		id                    int
		name                  string
		slot, attack, raceuse int
	}{
		{1297, "Apprentice Sword", 12, 1, 1023}, {5488, "Apprentice Club", 12, 3, 1023}, {5489, "Apprentice Staff", 15, 4, 1023},
		{26435, "toad snacks", -1, 0, 1023}, {8398, "Field Rations", -1, 0, 1023}, {715000, "Genie's Lamp", -1, 0, 32767},
	} {
		testPattern(t, db, p.id, p.name, p.slot, p.attack, p.raceuse)
	}
	return db, s
}

// testPattern makes an itempattern row: every class's, level 1.
func testPattern(t *testing.T, db *SQLiteDB, id int, name string, slot, attack, raceuse int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO itempattern
		(patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (?, 0, 0, 0, 0, ?, 0, 0, 0, 0, ?, 0, 0, 1, 1, 100, 0, 32767, ?, 0, 0, 0, 0, ?, '', 0, 0)`,
		id, slot, attack, raceuse, name); err != nil {
		t.Fatal(err)
	}
}

// A race of racedefs starts on its own rows; one with none on its wire race's
// rows (a human-passing race's city's), named by its wire race's city; a human
// of a class its city has no row for stays in its own city (the east's warrior's
// spot, not the west's paladin's), and a class with no row anywhere takes its
// race's first.
func TestStartPlaceOfARaceOfRacedefs(t *testing.T) {
	_, s := creationDB(t)
	cases := []struct {
		name             string
		race, class, hum int
		x, z             float32
		world            int
		city             string
	}{
		{"Giant", 11, 0, 0, 9300, 7300, 1, "Oggok"},            // its own row
		{"Giantmage", 11, 13, 0, 9300, 7300, 1, "Oggok"},       // its own rows' first
		{"Kitten", 12, 1, 0, 9005, 12749, 0, "Surefall Glade"}, // no rows: the west's ranger's
		{"Kat", 12, 8, 0, 4400, 17100, 0, "Qeynos"},            // the west's shaman's
		{"Katsk", 12, 3, 0, 4243.7, 17169.1, 0, "Qeynos"},      // the west has no SK: its first, not the east's SK
		{"Eastpal", 0, 2, 1, 25486.5, 15547.0, 0, "Freeport"},  // the east has no paladin: its warrior's
		{"Ribbitranger", raceFroglok, 1, 0, 25486.5, 15547.0, 0, "Freeport"},
		{"Trollwiz", 5, 13, 0, 25408, 31410, 0, "Grobb"},
	}
	for _, c := range cases {
		r := &CharacterRecord{CharName: c.name, AccountID: 1, Race: c.race, TClass: c.class, HumType: c.hum}
		if _, err := s.CreateNew(r); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, _ := s.GetByServerID(r.ServerID)
		b, ok, _ := s.LoadBind(r.ServerID)
		if got.X != c.x || got.Z != c.z || got.World != c.world || !ok || b.Spiritmaster != c.city {
			t.Errorf("%s: at (%.1f, %.1f) world %d, bound by %q, want (%.1f, %.1f) world %d by %q",
				c.name, got.X, got.Z, got.World, b.Spiritmaster, c.x, c.z, c.world, c.city)
		}
	}
	// the control: a race with no racedefs row and no start row stays at the origin
	r := &CharacterRecord{CharName: "Nobody", AccountID: 1, Race: 13, TClass: 0}
	if _, err := s.CreateNew(r); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetByServerID(r.ServerID); got.X != 0 || got.Z != 0 {
		t.Errorf("a race with no row moved to (%.1f, %.1f)", got.X, got.Z)
	}
}

// A combo with start items is as its rows; one without carries its race's but
// their weapon, and its class's weapon (the one most combos of the class carry; a
// monk none); a race of racedefs with no rows its wire race's (a human-passing
// race's city's); a race with no rows anywhere nothing.
func TestStartItemsOfACombo(t *testing.T) {
	db, _ := creationDB(t)
	got := func(race, class, hum int) string {
		t.Helper()
		items, err := db.StartItems(race, class, hum)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(items)
	}
	for _, c := range []struct {
		name             string
		race, class, hum int
		want             string
	}{
		{"a troll warrior, as its rows", 5, 0, 0, "[{26435 99} {1297 1} {715000 1}]"},
		{"a troll wizard: snacks, the lamp, a staff", 5, 13, 0, "[{26435 99} {715000 1} {5489 1}]"},
		{"a troll monk: no weapon", 5, 4, 0, "[{26435 99} {715000 1}]"},
		{"a troll cleric: a club", 5, 9, 0, "[{26435 99} {715000 1} {5488 1}]"},
		{"a western human warrior: the west's rations and a sword", 0, 0, 2, "[{8398 99} {715000 1} {1297 1}]"},
		{"a cat folk magician, the west's own row", 12, 12, 0, "[{8398 99} {5489 1} {715000 1}]"},
		{"a froglok magician, the east's row", raceFroglok, 10, 0, "[{8398 99} {5489 1} {715000 1}]"},
		{"a giant with no rows: none to take (the ogre has none here)", 11, 0, 0, "[]"},
		{"a race of no row anywhere", 14, 0, 0, "[]"},
	} {
		if g := got(c.race, c.class, c.hum); g != c.want {
			t.Errorf("%s: %s, want %s", c.name, g, c.want)
		}
	}
	// the control: a combo's own rows win over the fallback
	if _, err := db.Exec(`INSERT INTO startitems (race, class, humtype, patternid, count) VALUES (5, 13, 0, 1297, 1)`); err != nil {
		t.Fatal(err)
	}
	if g := got(5, 13, 0); g != "[{1297 1}]" {
		t.Errorf("a combo with a row: %s", g)
	}
}

// The froglok's race (10) is in a mask naming every race of the game's, the
// data's "every race" (1023), as raceUseForWire tells clients: its heirlooms and
// template take the items of every race. A race of the game's is as its bit.
func TestWearablePatternsTakeTheFroglokInEveryRace(t *testing.T) {
	db, _ := creationDB(t)
	testPattern(t, db, 900001, "every race", 1, 0, 1023)
	testPattern(t, db, 900002, "trolls only", 1, 0, 32)
	testPattern(t, db, 900003, "all but trolls", 1, 0, 991)
	testPattern(t, db, 900004, "frogloks only", 1, 0, 1024)
	inv := NewInventoryStore(db)
	names := func(race int) string {
		t.Helper()
		items, err := inv.WearablePatterns(0, race, 60, []int{1})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range items {
			out = append(out, it.ItemName)
		}
		return fmt.Sprint(out)
	}
	if g := names(raceFroglok); g != "[every race frogloks only]" {
		t.Errorf("the froglok: %s", g)
	}
	if g := names(5); g != "[every race trolls only]" {
		t.Errorf("a troll: %s", g)
	}
	if g := names(4); g != "[every race all but trolls]" {
		t.Errorf("the control, a dwarf: %s", g)
	}
}

// A racedefs table made before creation's columns gains them, each as declared,
// its rows kept; one made with them is left as it is.
func TestRaceDefsGainCreationsColumns(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	// the table as the race-label binary made it
	for _, q := range []string{`DROP TABLE racedefs`, `CREATE TABLE racedefs (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL, short_name TEXT NOT NULL DEFAULT '', wire_race INTEGER NOT NULL,
		bonus_str INTEGER NOT NULL DEFAULT 0, bonus_sta INTEGER NOT NULL DEFAULT 0, bonus_agi INTEGER NOT NULL DEFAULT 0,
		bonus_dex INTEGER NOT NULL DEFAULT 0, bonus_wis INTEGER NOT NULL DEFAULT 0, bonus_int INTEGER NOT NULL DEFAULT 0,
		bonus_cha INTEGER NOT NULL DEFAULT 0, body_male INTEGER NOT NULL DEFAULT 0, body_female INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm')`,
		`INSERT INTO racedefs (id, name, short_name, wire_race, bonus_str, body_male, body_female) VALUES (11, 'Hill Giant', 'HGT', 9, 25, -5117396, -5117396)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.RaceDefRows()
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v, %v", rows, err)
	}
	r := rows[0]
	if r.Name != "Hill Giant" || r.Bonus[0] != 25 || r.BodyMale != -5117396 || r.Desc != "" || r.ClassMask != 0 || r.City != 0 {
		t.Errorf("the row after: %+v", r)
	}
	if _, err := db.Exec(`UPDATE racedefs SET description = 'big', class_mask = 9, city = 0 WHERE id = 11`); err != nil {
		t.Fatalf("the new columns: %v", err)
	}
	// a fresh table has them in the same order: the control
	var cols []string
	q, err := db.Query(`SELECT name FROM pragma_table_info('racedefs') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	for q.Next() {
		var n string
		q.Scan(&n)
		cols = append(cols, n)
	}
	q.Close()
	if fmt.Sprint(cols[len(cols)-3:]) != "[description class_mask city]" {
		t.Errorf("columns %v", cols)
	}
}
