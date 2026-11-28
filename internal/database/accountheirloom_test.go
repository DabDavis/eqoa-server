package database

import (
	"testing"
	"time"
)

// twoOfOneAccount makes two characters of account 7 (and one of account 8) and is their serverids.
func twoOfOneAccount(t *testing.T, db *SQLiteDB) (a, b, other uint32) {
	t.Helper()
	for _, c := range []struct {
		id   uint32
		acct int
		name string
	}{{101, 7, "Alpha"}, {102, 7, "Bravo"}, {103, 8, "Other"}} {
		if _, err := db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor,
				hairlength, hairstyle, faceoption, sex, world, x, y, z) VALUES (?, ?, ?, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
			c.name, c.acct, c.id); err != nil {
			t.Fatal(err)
		}
	}
	return 101, 102, 103
}

func rankOf(t *testing.T, db *SQLiteDB, id uint32) int {
	t.Helper()
	m, err := db.CharMarksOf(id)
	if err != nil {
		t.Fatal(err)
	}
	return m.HeirloomRank
}

// A rank one character buys is its whole account's (Heirloom petition 3415): the other character
// of the account reads it, a character of another account does not, and the next purchase on the
// second character starts from the account's rank (no rank paid for twice).
func TestHeirloomRankIsTheAccounts(t *testing.T) {
	db := marksDB(t)
	a, b, other := twoOfOneAccount(t, db)
	now := time.Unix(1_800_000_000, 0)
	if _, _, _, err := db.AwardMarks(a, MarksAward{Source: "gm", Base: 500, Bonus: 1}, now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.AwardMarks(b, MarksAward{Source: "gm", Base: 500, Bonus: 1}, now); err != nil {
		t.Fatal(err)
	}
	buy := func(id uint32) CharMarks {
		t.Helper()
		m, err := db.SpendMarks(id, 100, "rank", func(c *CharMarks) error { c.HeirloomRank++; return nil }, now)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	if got := buy(a).HeirloomRank; got != 1 {
		t.Fatalf("A bought rank %d, want 1", got)
	}
	if got := rankOf(t, db, b); got != 1 {
		t.Errorf("B has rank %d after A's purchase, want 1 (the account's)", got)
	}
	if got := rankOf(t, db, other); got != 0 {
		t.Errorf("another account's character has rank %d, want 0", got)
	}
	if got := buy(b).HeirloomRank; got != 2 {
		t.Errorf("B's purchase made rank %d, want 2 (from the account's 1)", got)
	}
	if got := rankOf(t, db, a); got != 2 {
		t.Errorf("A reads rank %d after B's purchase, want 2", got)
	}
	if m, _ := db.CharMarksOf(b); m.Marks != 400 {
		t.Errorf("B's marks %d, want 400: only B paid for its rank", m.Marks)
	}
}

// The migration takes each account's highest rank its characters bought, and never lowers one.
func TestAccountHeirloomMigrationTakesTheMax(t *testing.T) {
	db := marksDB(t)
	a, b, other := twoOfOneAccount(t, db)
	for _, r := range []struct {
		id   uint32
		rank int
	}{{a, 2}, {b, 4}, {other, 1}} {
		if _, err := db.Exec(`INSERT INTO charmarks (serverid, heirloom_rank) VALUES (?, ?)`, r.id, r.rank); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO accountheirloom (accountid, rank) VALUES (8, 3)`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateAccountHeirloom(); err != nil {
		t.Fatal(err)
	}
	var r7, r8 int
	db.QueryRow(`SELECT rank FROM accountheirloom WHERE accountid = 7`).Scan(&r7)
	db.QueryRow(`SELECT rank FROM accountheirloom WHERE accountid = 8`).Scan(&r8)
	if r7 != 4 || r8 != 3 {
		t.Errorf("accounts' ranks %d and %d, want 4 (the max of 2 and 4) and 3 (kept over its character's 1)", r7, r8)
	}
	if got := rankOf(t, db, a); got != 4 {
		t.Errorf("A reads %d, want its account's 4", got)
	}
}
