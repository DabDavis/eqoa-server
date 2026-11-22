package database

import (
	"testing"
	"time"
)

// Every Ascended record is NO TRADE (work board 1714): the copy of a trade-0
// base is trade 1 when made and after a sync, the base still 0; a Legacy
// record of it is the record but its name and its trade (0), its use effects
// copied, and after the record changes and a start's sync it is the record
// again, still "Legacy " and trade 0. The control: a record with no Legacy row
// is left alone.
func TestAscendedIsNoTradeAndLegacyKeepsTrading(t *testing.T) {
	db := ascendedDB(t)
	now := time.Unix(1_800_000_000, 0)
	helm, _, err := ascendedCopy(t, db, 6207)
	if err != nil {
		t.Fatal(err)
	}
	trade := func(id int) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT trade FROM itempattern WHERE patternid = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if trade(helm) != 1 || trade(6207) != 0 {
		t.Fatalf("made: the copy's trade %d (want 1), the base's %d (want 0)", trade(helm), trade(6207))
	}
	if _, err := db.Exec(`UPDATE itempattern SET trade = 0 WHERE patternid = ?`, helm); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAscendedCopies(db, now); err != nil {
		t.Fatal(err)
	}
	if trade(helm) != 1 || trade(6207) != 0 {
		t.Fatalf("synced: the copy's trade %d (want 1), the base's %d (want 0)", trade(helm), trade(6207))
	}

	const legacy = 967001
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := LegacyCopyTx(tx, legacy, helm, "test", now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	check := func(when string, str int) {
		t.Helper()
		var name, line string
		var s int
		if err := db.QueryRow(`SELECT itemname, str FROM itempattern WHERE patternid = ?`, legacy).Scan(&name, &s); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT line FROM itemeffect WHERE patternid = ?`, legacy).Scan(&line); err != nil {
			t.Fatalf("%s: its use effect: %v", when, err)
		}
		var want string
		if err := db.QueryRow(`SELECT line FROM itemeffect WHERE patternid = ?`, helm).Scan(&want); err != nil {
			t.Fatal(err)
		}
		if name != "Legacy Ascended Otherworldly Helm" || trade(legacy) != 0 || s != str || line != want {
			t.Errorf("%s: %q trade %d str %d line %q; want the Legacy name, trade 0, str %d, %q", when, name, trade(legacy), s, line, str, want)
		}
	}
	var str int
	if err := db.QueryRow(`SELECT str FROM itempattern WHERE patternid = ?`, helm).Scan(&str); err != nil {
		t.Fatal(err)
	}
	check("made", str)
	// the base changes: its Ascended record and the Legacy record follow at a start
	if _, err := db.Exec(`UPDATE itempattern SET str = 40 WHERE patternid = 6207`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE itempattern SET trade = 1, itemname = 'x' WHERE patternid = ?`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAscendedCopies(db, now); err != nil {
		t.Fatal(err)
	}
	if n, err := SyncLegacyCopies(db, now); err != nil || n != 1 {
		t.Fatalf("synced %d Legacy records (%v), want 1", n, err)
	}
	check("after a start", AscendedFigure(40))
	m, err := LegacyMap(db)
	if err != nil || len(m) != 1 || m[legacy] != helm {
		t.Errorf("the map %v (%v)", m, err)
	}
	if LegacyBaseName("legacy Ascended Otherworldly Helm") != "Ascended Otherworldly Helm" || LegacyBaseName("Steelwind") != "Steelwind" {
		t.Error("LegacyBaseName")
	}
	// the control: a record without a Legacy row is not touched
	if trade(1026) != 0 {
		t.Errorf("Steelwind's trade %d, want its own 0", trade(1026))
	}
}
