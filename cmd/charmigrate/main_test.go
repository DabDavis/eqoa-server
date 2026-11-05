package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// migrateDB is a database with the server's whole schema, a level 4 shadow
// knight on it with an item in its pack and its hit points saved, three
// records (one a class mask of 0, one to copy), and its class's list: a spell
// a level gives, a mastery's, a were form, and one it holds already.
func migrateDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.db")
	s, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (81, 'player', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level,
			haircolor, hairlength, hairstyle, faceoption, sex, world, x, y, z, tunar, totalXP)
			VALUES ('Wanderer', 81, 126, 1, 3, 5, 1, 4, 0, 0, 0, 0, 0, 0, 25000, 60, 31000, 4730, 15000)`,
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color, AC)
			VALUES (1297, 0, 0, 1, 0, 12, 0, 0, 1, 0, 1, 10, 0, 1, 1, 200, 100, 32767, 1023, 0, 0, 0, 0, 'Apprentice Sword', '', 7, 0, 0),
			       (6207, 10, 0, 2, 0, 1, 0, 0, 1, 0, 0, 0, 0, 55, 1, 9000, 50, 0, 1023, 0, 0, 0, 0, 'Otherworldly Helm', '', 0, 0, 130),
			       (5408, 1456, 0, 3, 0, 15, 0, 0, 1, 0, 2, 789, 0, 60, 1, 10578, 50, 12, 1023, 0, 0, 0, 0, 'naginata of toxicity', 'It reeks of poison.', 1739852516, 0, 0)`,
		`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (126, 1, 200, 0, 1297, 14, 0, 0)`,
		`INSERT INTO charvitals (serverid, hp, power, saved) VALUES (126, 153, 40, '2026-11-05T02:16:29Z')`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
			VALUES (11, 1, 0, 0, 0, 2, 5, 0, 1, 1, 3, 0, 'Suffering', ''),
			       (12, 30, 0, 0, 0, 2, 50, 0, 1, 1, 3, 0, 'Dark Shroud', ''),
			       (13, 60, 0, 0, 0, 2, 50, 0, 1, 1, 3, 0, 'Mastered Shroud', ''),
			       (14, 20, 0, 0, 0, 2, 50, 0, 1, 1, 3, 0, 'Werewolf Form', 'Turns you into a werewolf.')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (3, 11, 'dump'), (3, 12, 'dump'), (3, 13, 'eqoa-data cm'), (3, 14, 'dump')`,
		`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide) VALUES (126, 11, 0, 0, -1, 0, -1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db, path
}

func testPlan() planFile {
	return planFile{
		Character: "wanderer", Level: 60, MasteryPoints: 1040, ClassSpells: true, FullVitals: true,
		Records:    []recordFix{{Pattern: 6207, Set: map[string]any{"classuse": float64(525)}, Why: "mask 0"}},
		NewRecords: []newRecord{{Ref: "nodachi", Like: 5408, Set: map[string]any{"itemname": "No-Dachi of Toxicity"}}},
		Items:      []planItem{{Pattern: 6207}, {Pattern: 1297, Count: 2}, {New: "nodachi"}},
	}
}

// dump is the rows a migration touches, for comparing a database before and
// after a run and its undo.
func dump(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT level, totalXP FROM characters WHERE serverid = 126`,
		`SELECT points, spent FROM charmastery WHERE serverid = 126`,
		`SELECT spellid, addedorder FROM spells WHERE serverid = 126 ORDER BY id`,
		`SELECT hp, power FROM charvitals WHERE serverid = 126`,
		`SELECT patternid, classuse, itemname FROM itempattern ORDER BY patternid`,
		`SELECT patternid, equiploc, location, listnumber FROM charinventory WHERE serverid = 126 ORDER BY itemid`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows.Scan(ptrs...)
			for _, v := range vals {
				b.WriteString(cell(v))
				b.WriteByte('|')
			}
			b.WriteByte('\n')
		}
		rows.Close()
		b.WriteString("--\n")
	}
	return b.String()
}

// cell is a column's value as text.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(x)
	}
	return fmt.Sprint(v)
}

// A dry run writes nothing; a run does the plan: the level and the
// experience that reaches it, the points, the class's spells a level gives
// (not the mastery's, not the were form, not one held) after what the book
// holds, the saved hit points gone, the mask set, a copy made with its name,
// the items in the bank after nothing (listnumbers from 0), the pack as it
// was. Undo puts every row back as it was.
func TestMigrateAndUndo(t *testing.T) {
	db, _ := migrateDB(t)
	before := dump(t, db)
	if err := migrate(db, testPlan(), false, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, db); got != before {
		t.Fatalf("a dry run wrote:\n%s\nwas\n%s", got, before)
	}
	if err := migrate(db, testPlan(), true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var level, xp, points int
	db.QueryRow(`SELECT level, totalXP FROM characters WHERE serverid = 126`).Scan(&level, &xp)
	db.QueryRow(`SELECT points FROM charmastery WHERE serverid = 126`).Scan(&points)
	if level != 60 || xp != entity.TotalXP(60, 0) || points != 1040 {
		t.Fatalf("level %d xp %d points %d", level, xp, points)
	}
	var spells []string
	rows, _ := db.Query(`SELECT p.spellname || ':' || s.addedorder FROM spells s JOIN spellpattern p ON p.spellid = s.spellid WHERE s.serverid = 126 ORDER BY s.addedorder`)
	for rows.Next() {
		var s string
		rows.Scan(&s)
		spells = append(spells, s)
	}
	rows.Close()
	if strings.Join(spells, ",") != "Suffering:0,Dark Shroud:1" {
		t.Fatalf("spells %v", spells)
	}
	var vitals int
	db.QueryRow(`SELECT COUNT(*) FROM charvitals WHERE serverid = 126`).Scan(&vitals)
	var mask int
	db.QueryRow(`SELECT classuse FROM itempattern WHERE patternid = 6207`).Scan(&mask)
	var newID, newDmg int
	var newName string
	db.QueryRow(`SELECT patternid, itemname, weapondamage FROM itempattern WHERE itemname = 'No-Dachi of Toxicity'`).Scan(&newID, &newName, &newDmg)
	if vitals != 0 || mask != 525 || newID <= 6207 || newDmg != 789 {
		t.Fatalf("vitals %d mask %d new %d %q %d", vitals, mask, newID, newName, newDmg)
	}
	var bank []string
	rows, _ = db.Query(`SELECT patternid || '@' || listnumber || '/' || equiploc FROM charinventory WHERE serverid = 126 AND location = 1 ORDER BY listnumber`)
	for rows.Next() {
		var s string
		rows.Scan(&s)
		bank = append(bank, s)
	}
	rows.Close()
	want := "6207@0/-1,1297@1/-1,1297@2/-1," + strconv.Itoa(newID) + "@3/-1"
	if strings.Join(bank, ",") != want {
		t.Fatalf("bank %v, want %s", bank, want)
	}
	var pack int
	db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = 126 AND location = 0`).Scan(&pack)
	if pack != 1 {
		t.Fatalf("pack %d", pack)
	}

	var run string
	db.QueryRow(`SELECT run FROM charmigrate_log LIMIT 1`).Scan(&run)
	if err := undoRun(db, run, false); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, db); got != before {
		t.Fatalf("undo left:\n%s\nwas\n%s", got, before)
	}
	if err := undoRun(db, run, false); err == nil {
		t.Fatal("a run undone twice")
	}
}

// Undo leaves a row changed since the run as it is (the control: the rest go
// back), unless forced.
func TestUndoLeavesWhatChangedSince(t *testing.T) {
	db, _ := migrateDB(t)
	if err := migrate(db, testPlan(), true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE characters SET level = 59 WHERE serverid = 126`) // a GM's change since
	var run string
	db.QueryRow(`SELECT run FROM charmigrate_log LIMIT 1`).Scan(&run)
	if err := undoRun(db, run, false); err != nil {
		t.Fatal(err)
	}
	var level, items int
	db.QueryRow(`SELECT level FROM characters WHERE serverid = 126`).Scan(&level)
	db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = 126`).Scan(&items)
	if level != 59 || items != 1 {
		t.Fatalf("level %d items %d: the changed level stays, the items go", level, items)
	}
	if err := undoRun(db, run, true); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT level FROM characters WHERE serverid = 126`).Scan(&level)
	if level != 4 {
		t.Fatalf("forced, level %d", level)
	}
}

// Moves take rows the character holds to the pack (unequipped, at the end of
// its list) and undo puts them back where they were; the control: a row
// already there is left, one the character does not hold refuses the plan.
func TestMovesAndUndo(t *testing.T) {
	db, _ := migrateDB(t)
	if err := migrate(db, testPlan(), true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var ids []int
	rows, _ := db.Query(`SELECT itemid FROM charinventory WHERE serverid = 126 AND location = 1 ORDER BY listnumber`)
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	before := dump(t, db)
	p := planFile{Character: "Wanderer", Moves: []planMove{{ItemID: ids[0], To: "pack"}, {ItemID: ids[1], To: "pack"}}}
	if err := migrate(db, p, true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	var got []string
	rows, _ = db.Query(`SELECT itemid || '@' || location || '/' || listnumber || '/' || equiploc FROM charinventory WHERE serverid = 126 AND location = 0 ORDER BY listnumber`)
	for rows.Next() {
		var s string
		rows.Scan(&s)
		got = append(got, s)
	}
	rows.Close()
	want := []string{"1@0/0/14", strconv.Itoa(ids[0]) + "@0/1/-1", strconv.Itoa(ids[1]) + "@0/2/-1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pack %v, want %v", got, want)
	}
	var run string
	db.QueryRow(`SELECT run FROM charmigrate_log WHERE kind = 'move' LIMIT 1`).Scan(&run)
	if err := undoRun(db, run, false); err != nil {
		t.Fatal(err)
	}
	if got := dump(t, db); got != before {
		t.Fatalf("undo left:\n%s\nwas\n%s", got, before)
	}
	p.Moves = []planMove{{ItemID: 99999, To: "pack"}}
	if err := migrate(db, p, true, false, io.Discard); err == nil || !strings.Contains(err.Error(), "holds no item") {
		t.Fatalf("an item not held: %v", err)
	}
}

// A plan is refused when it names no character here, a pack it would
// overfill, or an item by both a pattern and a new record.
func TestMigrateRefuses(t *testing.T) {
	db, _ := migrateDB(t)
	p := testPlan()
	p.Character = "Nobody"
	if err := migrate(db, p, true, false, io.Discard); err == nil || !strings.Contains(err.Error(), "no character") {
		t.Fatalf("no character: %v", err)
	}
	p = testPlan()
	p.To = "pack"
	p.Items = []planItem{{Pattern: 1297, Count: 20}, {Pattern: 1297, Count: 20}}
	if err := migrate(db, p, true, false, io.Discard); err == nil || !strings.Contains(err.Error(), "pack holds") {
		t.Fatalf("full pack: %v", err)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "p.json")
	os.WriteFile(bad, []byte(`{"character": "Wanderer", "items": [{"pattern": 1297, "new": "x"}]}`), 0o600)
	if _, err := readPlan(bad); err == nil {
		t.Fatal("an item by both a pattern and a new record")
	}
}

// The log decides offline by the latest login's object: in the world after
// its selection, out once that object's session is cleaned up, in again after
// a new login; a log without a login of it is not an answer.
func TestOfflineInLog(t *testing.T) {
	dir := t.TempDir()
	write := func(lines ...string) string {
		p := filepath.Join(dir, "gameserver.log")
		os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
		return p
	}
	login := []string{
		"2026/11/05 02:04:47 SelectedChar: received 20 bytes from 1.2.3.4:5",
		"2026/11/05 02:04:47 SelectedChar: serverID=126 from 1.2.3.4:5",
		"2026/11/05 02:04:47 SelectedChar: selected 'Wanderer' (level 4 race=5 class=3)",
		"2026/11/05 02:04:47 SelectedChar: generated playerObjectID=500000008 (0x1DCD6508)",
	}
	other := "2026/11/05 02:10:00 Cleaning up terminated session 1.2.3.4:6 (instance=0x1, objectID=500000001)"
	mine := "2026/11/05 02:20:00 Cleaning up terminated session 1.2.3.4:5 (instance=0x28ED7DFA, objectID=500000008)"
	cases := []struct {
		lines       []string
		off, ok     bool
		description string
	}{
		{nil, false, false, "no login"},
		{append(append([]string{}, login...), other), false, true, "another session ended"},
		{append(append([]string{}, login...), other, mine), true, true, "its session ended"},
		{append(append(append([]string{}, login...), mine), strings.ReplaceAll(strings.Join(login, "\n"), "500000008", "500000009")), false, true, "back in"},
	}
	for _, c := range cases {
		lines := c.lines
		if lines == nil {
			lines = []string{other}
		}
		off, ok, _, err := offlineInLog(write(lines...), "wanderer")
		if err != nil || off != c.off || ok != c.ok {
			t.Errorf("%s: offline %v ok %v err %v", c.description, off, ok, err)
		}
	}
}
