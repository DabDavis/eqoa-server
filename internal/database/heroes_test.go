package database

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

// The Testing Heroes' tables (heroes.go; petition 1471). The names and
// figures are made up.

func heroesDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/heroes.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// The stock is read cheapest first, rows of no cost left out; a record is
// sold once (a second row of it is refused).
func TestHeroesStockIsReadCheapestFirst(t *testing.T) {
	db := heroesDB(t)
	if rows, err := db.HeroesStock(); err != nil || len(rows) != 0 {
		t.Fatalf("a new database's stock: %v %v", rows, err)
	}
	for _, r := range [][3]any{{501, 250, "the weakened one"}, {502, 25, ""}, {503, 1000, ""}, {504, 0, "not for sale"}, {505, 25, "second of 25"}} {
		if _, err := db.Exec(`INSERT INTO heroes_stock (patternid, cost, says) VALUES (?, ?, ?)`, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.HeroesStock()
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, r := range rows {
		got = append(got, r.PatternID)
	}
	if len(got) != 4 || got[0] != 502 || got[1] != 505 || got[2] != 501 || got[3] != 503 {
		t.Errorf("the stock in order: %v, want [502 505 501 503]", got)
	}
	if rows[2].Says != "the weakened one" || rows[2].Cost != 250 || rows[2].ID == 0 {
		t.Errorf("the row: %+v", rows[2])
	}
	if _, err := db.Exec(`INSERT INTO heroes_stock (patternid, cost) VALUES (501, 5)`); err == nil {
		t.Error("a record was stocked twice")
	}
	var source string
	if err := db.QueryRow(`SELECT source FROM heroes_stock WHERE patternid = 501`).Scan(&source); err != nil || source != "gm" {
		t.Errorf("a row's source %q (%v), want gm", source, err)
	}
}

// A purchase is kept as it was made.
func TestAHeroesPurchaseIsLogged(t *testing.T) {
	db := heroesDB(t)
	at := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC)
	p := HeroesPurchase{At: at, ServerID: 640, CharName: "Heroanna", AccountID: 202, PatternID: 501, Item: "Weakened Breastplate of the Anointed",
		Cost: 250, StandingBefore: -1700, StandingAfter: -1950, Vendor: "Testing Heroes Vendor"}
	if err := db.LogHeroesPurchase(p); err != nil {
		t.Fatal(err)
	}
	var got HeroesPurchase
	var when string
	if err := db.QueryRow(`SELECT at, serverid, charname, accountid, patternid, item, cost, standing_before, standing_after, vendor
		FROM heroes_purchases`).Scan(&when, &got.ServerID, &got.CharName, &got.AccountID, &got.PatternID, &got.Item, &got.Cost,
		&got.StandingBefore, &got.StandingAfter, &got.Vendor); err != nil {
		t.Fatal(err)
	}
	got.At = at
	if got != p || when != "2026-11-10T22:00:00Z" {
		t.Errorf("kept %+v at %q, want %+v", got, when, p)
	}
}

// A character's bonus training points: none with no row, kept with its
// unspent and total in one write, gone with the character; a character that
// is not there is refused and nothing is written.
func TestBonusTrainingPointsAreKeptWithTheCharacter(t *testing.T) {
	db := heroesDB(t)
	store := NewSQLiteCharacterStore(db)
	rec := &CharacterRecord{CharName: "Reader", AccountID: 1, Level: 10}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	if n, err := store.BonusTP(rec.ServerID); err != nil || n != 0 {
		t.Fatalf("no row: %d %v", n, err)
	}
	at := time.Date(2026, 11, 10, 22, 0, 0, 0, time.UTC)
	if err := store.SetBonusTraining(rec.ServerID, 12, 70, 5, at); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBonusTraining(rec.ServerID, 17, 75, 10, at); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByServerID(rec.ServerID)
	if err != nil || got.UnusedTP != 17 || got.TotalTP != 75 {
		t.Fatalf("the character: %+v %v, want 17 unspent of 75", got, err)
	}
	if n, err := store.BonusTP(rec.ServerID); err != nil || n != 10 {
		t.Errorf("bonus %d %v, want 10", n, err)
	}
	if err := db.SetBonusTP(rec.ServerID, -3, at); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.BonusTP(rec.ServerID); n != 0 {
		t.Errorf("a bonus below nothing kept as %d", n)
	}
	if err := store.SetBonusTraining(99999, 1, 1, 1, at); !errors.Is(err, ErrCharacterNotFound) {
		t.Errorf("no such character: %v", err)
	}
	if n, _ := db.BonusTP(99999); n != 0 {
		t.Errorf("no such character was given %d", n)
	}
	if err := db.SetBonusTP(rec.ServerID, 5, at); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(rec.ServerID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM charbonustp`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("after the character's deletion: %d rows (%v)", rows, err)
	}
}

// RaidBossNames is the same two rules as RaidBoss, whole: every name it gives
// is a raid boss to RaidBoss, and no other creature of the tables is.
func TestRaidBossNamesAreRaidBosss(t *testing.T) {
	db := heroesDB(t)
	for _, q := range []string{
		`INSERT INTO lootcreature (npc_name, region, profile) VALUES ('lady vox', 'permafrost', ' Boss '), ('a rat', 'freeport', 'animal'), ('skahyir', '', 'boss')`,
		`INSERT INTO npcloot (npc_name, tablename) VALUES ('lord nagafen', 'Boss nagafen'), ('a froglok', 'frogloks'), ('a boss mummy', 'bosses'), ('skahyir', 'boss skahyir')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	names, err := db.RaidBossNames()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	if got := strings.Join(names, "|"); got != "lady vox|lord nagafen|skahyir" {
		t.Errorf("the names %q", got)
	}
	inv := NewInventoryStore(db)
	is := map[string]bool{}
	for _, n := range names {
		is[n] = true
	}
	for _, n := range []string{"lady vox", "Lady Vox", "lord nagafen", "skahyir", "a rat", "a froglok", "a boss mummy", "nobody"} {
		boss, err := inv.RaidBoss(n)
		if err != nil {
			t.Fatal(err)
		}
		if boss != is[NPCClassKey(n)] {
			t.Errorf("%q: RaidBoss says %v, RaidBossNames %v", n, boss, is[NPCClassKey(n)])
		}
	}
}
