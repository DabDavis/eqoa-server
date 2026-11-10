package main

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/charmove"
	"github.com/eqoa/server/internal/database"
)

// damperDB is the server's schema with a transfer's run (a sword and a potion
// given, a second sword given and since sold), a GM's grant (a helm) and a
// run named to skip (a sword), as charmigrate leaves them.
func damperDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "d.db")
	s, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", path+"?"+database.BusyTimeoutDSN)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	item := func(id int, pattern int) string {
		return `INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (` + itoa(id) + `, 7, 1, 100, 0, ` + itoa(pattern) + `, -1, 1, ` + itoa(id) + `)`
	}
	given := func(run string, id, pattern int) string {
		return `INSERT INTO charmigrate_log (run, serverid, charname, kind, tbl, row_id, before, after, note, at)
			VALUES ('` + run + `', 7, 'Hero', 'item', 'charinventory', ` + itoa(id) + `, '', '{"patternid":` + itoa(pattern) + `}', '', 'now')`
	}
	for _, q := range []string{
		charmove.LogSchema,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color, AC)
			VALUES (1026, 0, 0, 1, 0, 14, 0, 0, 1, 0, 1, 581, 0, 49, 1, 100, 0, 1, 1023, 0, 0, 0, 0, 'Steelwind', '', 0, 0, 0),
			       (6207, 0, 0, 1, 0, 1, 0, 0, 1, 0, 0, 0, 0, 55, 1, 100, 0, 1, 1023, 0, 0, 0, 0, 'Otherworldly Helm', '', 0, 0, 130),
			       (50001, 0, 0, 1, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 1, 1023, 0, 0, 0, 0, 'Unstable Potion', '', 0, 0, 0)`,
		item(1, 1026), item(2, 50001), item(4, 6207), item(5, 1026),
		given("run-a", 1, 1026), given("run-a", 2, 50001), given("run-a", 3, 1026),
		given("run-grant", 4, 6207), given("run-skip", 5, 1026),
		`INSERT INTO chartransfers (accountid, serverid, charname, from_server, from_name, status, filed, updated, plan, run)
			VALUES (3, 7, 'Hero', 'GM grant', 'Hero', 'applied', 'now', 'now', '{}', 'run-grant')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db
}

func itoa(n int) string { return strconv.Itoa(n) }

func patterns(t *testing.T, db *sql.DB) map[int]int {
	t.Helper()
	rows, err := db.Query(`SELECT itemid, patternid FROM charinventory`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	m := map[int]int{}
	for rows.Next() {
		var id, p int
		rows.Scan(&id, &p)
		m[id] = p
	}
	return m
}

func run(t *testing.T, db *sql.DB, f func(*sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, s := range []string{logSchema, database.ImportedSchema} {
		if _, err := tx.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := f(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// The transfer's sword goes on Steelwind's copy; its potion (no gear), the
// sword it sold, the GM's grant and the run named to skip stay as they are;
// a second run moves nothing more, and the undo puts the sword back on
// Steelwind and takes the copy out.
func TestTheDamperMovesATransfersGearAndUndoesIt(t *testing.T) {
	defer database.SetImportedShare(database.SetImportedShare(0.7)) // the figures below are at 70%
	db := damperDB(t)
	now := time.Unix(1_800_000_000, 0)
	run(t, db, func(tx *sql.Tx) error { return move(tx, skipRuns("run-skip"), now) })
	got := patterns(t, db)
	var copyID, base int
	if err := db.QueryRow(`SELECT patternid, base FROM itemimported`).Scan(&copyID, &base); err != nil || base != 1026 {
		t.Fatalf("copies: %d of %d (%v), want Steelwind's alone", copyID, base, err)
	}
	if got[1] != copyID || got[2] != 50001 || got[4] != 6207 || got[5] != 1026 {
		t.Fatalf("after the move: %v; want row 1 on %d, the rest as they were", got, copyID)
	}
	var dmg int
	if err := db.QueryRow(`SELECT weapondamage FROM itempattern WHERE patternid = ?`, copyID).Scan(&dmg); err != nil || dmg != 407 {
		t.Errorf("the copy's damage %d (%v), want 407", dmg, err)
	}
	run(t, db, func(tx *sql.Tx) error { return move(tx, skipRuns("run-skip"), now) })
	var logged int
	if err := db.QueryRow(`SELECT count(*) FROM importdamper_log`).Scan(&logged); err != nil || logged != 1 {
		t.Errorf("a second run logged %d moves in all (%v), want 1", logged, err)
	}
	run(t, db, func(tx *sql.Tx) error { return undoMoves(tx, now) })
	if got := patterns(t, db); got[1] != 1026 {
		t.Errorf("after the undo: row 1 on %d, want 1026", got[1])
	}
	var copies, records int
	db.QueryRow(`SELECT count(*) FROM itemimported`).Scan(&copies)
	db.QueryRow(`SELECT count(*) FROM itempattern WHERE patternid = ?`, copyID).Scan(&records)
	if copies != 0 || records != 0 {
		t.Errorf("after the undo: %d copies listed, the copy's record %d times", copies, records)
	}
}
