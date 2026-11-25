package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A mark is a petition's feature once in any case; a petition that is not there is refused, a feature with no name
// too. The tallies are each account's features, each once in any case and the first adopted first, two accounts with
// the same feature both having it, named by the character that filed most of the account's Bug Report petitions; a
// database with no table has none.
func TestIdeaTallies(t *testing.T) {
	db, err := NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`INSERT INTO accountinfo (accountid, username) VALUES (5, 'poo'), (6, 'lifeispain')`)
	file := func(acct int, char string, cat, sub int) int64 {
		res, err := db.Exec(`INSERT INTO petitions (serverid, charname, accountid, category, subcategory, filed) VALUES (1, ?, ?, ?, ?, '2026-11-25T00:00:00Z')`,
			char, acct, cat, sub)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	a1, a2, a3 := file(5, "Cucumber", 1, 100), file(5, "Cucumber", 1, 100), file(5, "Spoony", 3, 30)
	b1 := file(6, "Waldorf", 1, 100)
	at := time.Date(2026, 11, 25, 4, 0, 0, 0, time.UTC)
	for _, m := range []struct {
		id      int64
		feature string
		new     bool
	}{{a2, "Quest Tracking", true}, {a1, "Auto Loot", true}, {a1, "AUTO LOOT", false}, {a3, " auto loot", true}, {b1, "Auto Loot", true}} {
		got, err := db.AdoptFeature(m.id, m.feature, "dab", at)
		if err != nil || got != m.new {
			t.Errorf("AdoptFeature(%d, %q): %v, %v; want %v", m.id, m.feature, got, err, m.new)
		}
		at = at.Add(time.Minute)
	}
	if _, err := db.AdoptFeature(99999, "x", "dab", at); err == nil {
		t.Error("a petition that is not there was marked")
	}
	if _, err := db.AdoptFeature(a1, "  ", "dab", at); err == nil {
		t.Error("a feature with no name was marked")
	}
	ts, err := IdeaTallies(db, FeatureRequestCategory)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range ts {
		got = append(got, x.Username+"/"+x.Player+":"+strings.Join(x.Features, ","))
	}
	if want := "poo/Cucumber:Quest Tracking,AUTO LOOT lifeispain/Waldorf:Auto Loot"; strings.Join(got, " ") != want {
		// MIN() picks a spelling: any one of a feature's is fine, so compare without case
		if !strings.EqualFold(strings.Join(got, " "), want) {
			t.Errorf("tallies %q, want %q", strings.Join(got, " "), want)
		}
	}
	if _, err := db.Exec(`DROP TABLE petitionadopted`); err != nil {
		t.Fatal(err)
	}
	if ts, err := IdeaTallies(db, FeatureRequestCategory); err != nil || ts != nil {
		t.Errorf("no table: %v, %v", ts, err)
	}
}
