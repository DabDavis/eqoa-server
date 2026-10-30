package database

import (
	"path/filepath"
	"testing"
	"time"
)

// A new database has every rehabilitator setting at its default (the user's
// prices); a GM's value is read, one below 0 counts as 0, a deleted setting
// is its default and comes back at the next start. The control: the
// defaults are the user's.
func TestRehabSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rehab.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := db.RehabSettings()
	if err != nil {
		t.Fatal(err)
	}
	want := RehabSettings{FreeMastery: 1, FreeTraining: 1, FreePath: 2, PriceMastery: 1_000_000, PriceTraining: 1_000_000,
		PriceBoth: 2_000_000, PriceRose: 10_000_000}
	if s != want || DefaultRehabSettings() != want {
		t.Fatalf("the defaults: %+v, want %+v", s, want)
	}
	db.Exec(`UPDATE rehabsettings SET value = 3 WHERE setting = 'free_path'`)
	db.Exec(`UPDATE rehabsettings SET value = -5 WHERE setting = 'price_rose'`)
	db.Exec(`DELETE FROM rehabsettings WHERE setting = 'price_both'`)
	if s, _ = db.RehabSettings(); s.FreePath != 3 || s.PriceRose != 0 || s.PriceBoth != 2_000_000 {
		t.Fatalf("after a GM's: %+v", s)
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM rehabsettings WHERE setting = 'price_both'`).Scan(&n)
	if s, _ = db.RehabSettings(); n != 1 || s.FreePath != 3 {
		t.Errorf("at the next start: price_both rows %d, free_path %d (a GM's kept)", n, s.FreePath)
	}
}

// A character's counts: none at first, a free reset used and one paid
// counted apart, a GM's bonus counted in what is left; a deleted character's
// counts go with it (a server id made again inherits none), another's stay.
func TestCharResetCounts(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "rehab.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewSQLiteCharacterStore(db)
	a, b := &CharacterRecord{CharName: "Anja", AccountID: 1}, &CharacterRecord{CharName: "Frejo", AccountID: 1}
	if err := store.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(b); err != nil {
		t.Fatal(err)
	}
	all, err := db.CharResets(a.ServerID)
	if err != nil || len(all) != 3 || all[ResetPath].Left(2) != 2 {
		t.Fatalf("at first: %+v %v", all, err)
	}
	now := time.Date(2026, 10, 30, 4, 0, 0, 0, time.UTC)
	db.CountReset(a.ServerID, ResetPath, true, now)
	db.CountReset(a.ServerID, ResetPath, false, now)
	db.CountReset(b.ServerID, ResetMastery, true, now)
	all, _ = db.CharResets(a.ServerID)
	if r := all[ResetPath]; r.Used != 1 || r.Paid != 1 || r.Left(2) != 1 || r.Updated != "2026-10-30T04:00:00Z" {
		t.Fatalf("counted: %+v", r)
	}
	db.Exec(`UPDATE charresets SET bonus = 2 WHERE serverid = ? AND kind = 'path'`, a.ServerID)
	all, _ = db.CharResets(a.ServerID)
	if all[ResetPath].Left(2) != 3 || all[ResetPath].Left(0) != 1 {
		t.Fatalf("with a bonus: %+v", all[ResetPath])
	}
	if err := store.Delete(a.ServerID); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM charresets WHERE serverid = ?`, a.ServerID).Scan(&n)
	if n != 0 {
		t.Errorf("a deleted character's counts: %d rows", n)
	}
	if all, _ := db.CharResets(b.ServerID); all[ResetMastery].Used != 1 {
		t.Errorf("the control: another's counts: %+v", all[ResetMastery])
	}
	for _, name := range []string{"rehabsettings", "setting", "value", "note", "charresets", "serverid", "kind", "used",
		"paid", "bonus", "updated", "charquests", "picks"} {
		if SecretLikeName(name) {
			t.Errorf("%q reads like a secret: every snapshot would be refused", name)
		}
	}
}

// A quest's picks are kept with its row: written, read back, a step's pick
// replaced; what cannot be read is left out.
func TestCharQuestPicks(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "picks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	qs := NewQuestStore(db)
	c := &CharQuest{ServerID: 5, QuestID: 100803, Done: 1, Picks: WithPick(WithPick("", 2, 1), 1, 3)}
	if c.Picks != "1:3, 2:1" {
		t.Fatalf("the picks: %q", c.Picks)
	}
	if err := qs.SaveCharQuest(c); err != nil {
		t.Fatal(err)
	}
	c.Picks = WithPick(c.Picks, 2, 0)
	qs.SaveCharQuest(c)
	rows, err := qs.CharQuests(5)
	if err != nil || len(rows) != 1 || rows[0].Picks != "1:3, 2:0" {
		t.Fatalf("read back: %+v %v", rows, err)
	}
	if p := ParsePicks("1:3, x:2, 4:-1, 2:0, 7"); len(p) != 2 || p[1] != 3 || p[2] != 0 {
		t.Errorf("parsed: %v", p)
	}
}
