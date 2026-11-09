package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// importedDB is the server's whole schema with four records: a sword and a
// helm (gear), a potion (no wear slot) and an Heirloom; the helm has a use
// effect.
func importedDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "i.db")
	s, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", path+"?"+BusyTimeoutDSN)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc,
			model, color, str, sta, AC, HPMAX, FR)
			VALUES (1026, 4201, 0, 5, 0, 14, 0, 0, 1, 0, 1, 581, 0, 49, 1, 200, 50, 1, 1023, 0, 0, 0, 0, 'Steelwind', 'A blade.', 402229003, -1, 15, NULL, 0, 70, -10),
			       (6207, 10, 0, 2, 0, 1, 0, 0, 1, 0, 0, 0, 0, 55, 1, 9000, 50, 32767, 1023, 0, 0, 0, 0, 'Otherworldly Helm', '', 9, 0, 1, 25, 130, 0, 0),
			       (50001, 5, 0, 3, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 0, 0, 0, 0, 'Unstable Potion', '', 0, 0, 0, 0, 0, 0, 0),
			       (714302, 0, 0, 4, 0, 14, 0, 0, 1, 0, 1, 581, 0, 1, 1, 200, 50, 1, 1023, 0, 0, 0, 0, 'Heirloom Steelwind', '', 402229003, -1, 15, 0, 0, 70, 0)`,
		`INSERT INTO itemeffect (patternid, line, name, icon, breaks, wear, area, source, body, target_line, charges, cooldown)
			VALUES (6207, 'Self, Increase AR by 75, Increase STR by 25, 30min', '', 0, 0, 0, 0, 'test', 0, '', 0, 60)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db
}

func copyOf(t *testing.T, db *sql.DB, base int) (int, bool) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	id, made, err := ImportedCopyTx(tx, base, "test", time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id, made
}

// Petition 1314: an import's gear is a copy of its record at 70% of its
// stats (rounded half away from zero, a penalty too, an empty one empty),
// its name, look and the rest its base's, made once a record, its use effect
// with each "by N" scaled. A potion and an Heirloom are no gear: given back
// as they are; a copy given again is itself.
func TestAnImportedCopyIsItsRecordAt70Percent(t *testing.T) {
	db := importedDB(t)
	id, made := copyOf(t, db, 1026)
	if !made || id != importedFirst {
		t.Fatalf("Steelwind's copy: %d (made %v), want %d made", id, made, importedFirst)
	}
	var name, desc string
	var dmg, str, ac, hp, fr, cost, model int
	var sta sql.NullInt64
	if err := db.QueryRow(`SELECT itemname, itemdesc, weapondamage, str, sta, AC, HPMAX, FR, itemcost, model FROM itempattern WHERE patternid = ?`, id).
		Scan(&name, &desc, &dmg, &str, &sta, &ac, &hp, &fr, &cost, &model); err != nil {
		t.Fatal(err)
	}
	if name != "Steelwind" || desc != "A blade." || dmg != 407 || str != 11 || sta.Valid || ac != 0 || hp != 49 || fr != -7 || cost != 4201 || model != 402229003 {
		t.Errorf("the copy: %q %q dmg %d str %d sta %v ac %d hp %d fr %d cost %d model %d; want Steelwind, dmg 407, str 11, sta NULL, hp 49, fr -7, the rest its base's",
			name, desc, dmg, str, sta, ac, hp, fr, cost, model)
	}
	if again, made := copyOf(t, db, 1026); again != id || made {
		t.Errorf("a second import of Steelwind: %d (made %v), want its copy %d", again, made, id)
	}
	if self, made := copyOf(t, db, id); self != id || made {
		t.Errorf("the copy given again: %d (made %v), want itself", self, made)
	}
	for _, base := range []int{50001, 714302} {
		if got, made := copyOf(t, db, base); got != base || made {
			t.Errorf("record %d (no gear): %d (made %v), want itself", base, got, made)
		}
	}
	helm, _ := copyOf(t, db, 6207)
	var line string
	var cooldown int
	if err := db.QueryRow(`SELECT line, cooldown FROM itemeffect WHERE patternid = ?`, helm).Scan(&line, &cooldown); err != nil {
		t.Fatal(err)
	}
	if line != "Self, Increase AR by 53, Increase STR by 18, 30min" || cooldown != 60 {
		t.Errorf("the helm's use effect: %q cooldown %d", line, cooldown)
	}
	m, err := ImportedMap(db)
	if err != nil || len(m) != 2 || m[id] != 1026 || m[helm] != 6207 {
		t.Errorf("ImportedMap: %v %v", m, err)
	}
}

// A record changed after its copies were made (a petition's new look, a GM's
// figure) takes them with it at the server's start; a copy as its base makes
// it is not written again. The control: an untouched copy stays.
func TestTheCopiesFollowTheirBasesAtTheStart(t *testing.T) {
	db := importedDB(t)
	sword, _ := copyOf(t, db, 1026)
	helm, _ := copyOf(t, db, 6207)
	if _, err := db.Exec(`UPDATE itempattern SET model = 612804823, str = 30, itemdesc = 'Now blue.' WHERE patternid = 1026`); err != nil {
		t.Fatal(err)
	}
	n, err := SyncImportedCopies(db, time.Unix(1_800_000_000, 0))
	if err != nil || n != 1 {
		t.Fatalf("synced %d (%v), want the sword's copy alone", n, err)
	}
	var model, str int
	var desc string
	if err := db.QueryRow(`SELECT model, str, itemdesc FROM itempattern WHERE patternid = ?`, sword).Scan(&model, &str, &desc); err != nil {
		t.Fatal(err)
	}
	if model != 612804823 || str != 21 || desc != "Now blue." {
		t.Errorf("the sword's copy after its base changed: model %d str %d %q", model, str, desc)
	}
	var ac int
	if err := db.QueryRow(`SELECT AC FROM itempattern WHERE patternid = ?`, helm).Scan(&ac); err != nil || ac != 91 {
		t.Errorf("the helm's copy: AC %d (%v), want 91", ac, err)
	}
	var effects int
	if err := db.QueryRow(`SELECT count(*) FROM itemeffect WHERE patternid = ?`, helm).Scan(&effects); err != nil || effects != 1 {
		t.Errorf("the helm's copy holds %d use effects (%v), want 1", effects, err)
	}
	if n, err := SyncImportedCopies(db, time.Unix(1_800_000_000, 0)); err != nil || n != 0 {
		t.Errorf("a second start synced %d (%v), want none", n, err)
	}
}
