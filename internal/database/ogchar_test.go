package database

import (
	"testing"
	"time"
)

// ogRig is a database with an [OG] account (two players) and a plain one
// (Bob), every character made before [OG] characters (og 0, not chosen).
func ogRig(t *testing.T) (*SQLiteDB, map[string]uint32, int) {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/og.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	accs := NewSQLiteAccountStore(db)
	og, err := accs.Create("ogowner", "x")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := accs.Create("plainowner", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetAccountOG(og.ID, true); err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteCharacterStore(db)
	ids := map[string]uint32{}
	for name, acc := range map[string]int{"Meshuggah": og.ID, "Riffy": og.ID, "Bob": plain.ID} {
		rec := CharacterRecord{CharName: name, AccountID: acc, Level: 5}
		if err := store.Create(&rec); err != nil {
			t.Fatal(err)
		}
		ids[name] = uint32(rec.ServerID)
	}
	return db, ids, og.ID
}

func ogCols(t *testing.T, db *SQLiteDB, id uint32) (og, chosen bool) {
	t.Helper()
	st, err := db.HardcoreOf(id)
	if err != nil {
		t.Fatal(err)
	}
	return st.OG, st.OGChosen
}

// The migration makes every character of an [OG] account an [OG] character,
// its choice made, and nothing else; it runs once: a second start changes
// nothing, and a character made since on the [OG] account stays standard
// (it chooses for itself).
func TestMigrateAccountOG(t *testing.T) {
	db, ids, ogAcc := ogRig(t)
	at := time.Unix(1_800_000_000, 0)
	m, err := db.MigrateAccountOG(true, at)
	if err != nil || !m.Ran || !m.Applied || m.Characters != 2 {
		t.Fatalf("first: %+v %v", m, err)
	}
	for name, want := range map[string]bool{"Meshuggah": true, "Riffy": true, "Bob": false} {
		if og, chosen := ogCols(t, db, ids[name]); og != want || chosen != want {
			t.Errorf("%s: og %v chosen %v, want %v", name, og, chosen, want)
		}
	}
	var logged int
	db.QueryRow(`SELECT COUNT(*) FROM og_char_migrated WHERE accountid = ? AND migrated_at = ?`, ogAcc, at.Unix()).Scan(&logged)
	if logged != 2 {
		t.Errorf("og_char_migrated: %d rows", logged)
	}
	// a character made since on the [OG] account; a GM made a player standard
	rec := CharacterRecord{CharName: "Newbie", AccountID: ogAcc, Level: 1}
	if _, err := NewSQLiteCharacterStore(db).CreateNew(&rec); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE characters SET og = 0 WHERE serverid = ?`, ids["Riffy"]); err != nil {
		t.Fatal(err)
	}
	m, err = db.MigrateAccountOG(true, at.Add(time.Hour))
	if err != nil || m.Ran || !m.Applied || m.Characters != 2 {
		t.Fatalf("again: %+v %v", m, err)
	}
	if og, _ := ogCols(t, db, uint32(rec.ServerID)); og {
		t.Error("the new character on the [OG] account was made [OG]")
	}
	if og, _ := ogCols(t, db, ids["Riffy"]); og {
		t.Error("the migration re-applied the account flag")
	}
	st, _ := db.HardcoreOf(uint32(rec.ServerID))
	if !st.OGChoiceOpen() {
		t.Errorf("the new character's choice is not open: %+v", st)
	}
	// its choice: taken once
	if ch, err := db.ChooseOG(uint32(rec.ServerID), true); !ch || err != nil {
		t.Fatalf("choose: %v %v", ch, err)
	}
	if ch, _ := db.ChooseOG(uint32(rec.ServerID), false); ch {
		t.Error("a choice made was changed")
	}
	if og, err := db.CharacterOG(uint32(rec.ServerID)); !og || err != nil {
		t.Errorf("CharacterOG: %v %v", og, err)
	}
	if _, err := db.CharacterOG(999999); err != ErrCharacterNotFound {
		t.Errorf("no character: %v", err)
	}
}

// On a shard where [OG] meant nothing (world.og_accounts off) the run is
// recorded and changes nothing, and a later start with it on does not take the
// account flag up.
func TestMigrateAccountOGNotApplied(t *testing.T) {
	db, ids, _ := ogRig(t)
	m, err := db.MigrateAccountOG(false, time.Now())
	if err != nil || !m.Ran || m.Applied || m.Characters != 0 {
		t.Fatalf("off: %+v %v", m, err)
	}
	m, err = db.MigrateAccountOG(true, time.Now())
	if err != nil || m.Ran || m.Applied {
		t.Fatalf("on later: %+v %v", m, err)
	}
	for name, id := range ids {
		if og, chosen := ogCols(t, db, id); og || chosen {
			t.Errorf("%s: og %v chosen %v", name, og, chosen)
		}
	}
}
