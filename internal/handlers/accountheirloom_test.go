package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Keeper prices an account's characters' next rank from the account's rank (Heirloom petition
// 3415): after A buys rank 1, B is offered rank 2 at its price, not rank 1 again; a character of
// another account is still offered rank 1; and Heirlooms reads B's rank as 1.
func TestKeeperPricesTheAccountsNextRank(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/k.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor,
			hairlength, hairstyle, faceoption, sex, world, x, y, z) VALUES
			('Alpha', 7, 101, 1, 0, 0, 1, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Bravo', 7, 102, 1, 0, 0, 1, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Other', 8, 103, 1, 0, 0, 1, 60, 0, 0, 0, 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	if _, err := db.SpendMarks(101, 0, "rank", func(c *database.CharMarks) error { c.HeirloomRank++; return nil }, now); err != nil {
		t.Fatal(err)
	}
	m := NewHeirloomMarks(db, nil, nil, nil, nil)
	offer := func(id uint32) (string, int) {
		t.Helper()
		row, err := db.CharMarksOf(id)
		if err != nil {
			t.Fatal(err)
		}
		label, price, why := m.offer(row, keeperRank)
		if why != "" {
			t.Fatalf("character %d: %q", id, why)
		}
		return label, price
	}
	if label, price := offer(102); price != heirloomRankPrices[1] || label != "Heirloom Rank 2, your heirlooms to level 51" {
		t.Errorf("B is offered %q at %d, want rank 2 at %d", label, price, heirloomRankPrices[1])
	}
	if _, price := offer(103); price != heirloomRankPrices[0] {
		t.Errorf("another account's character is offered rank at %d, want rank 1's %d", price, heirloomRankPrices[0])
	}
	SetHeirloomMarks(true)
	t.Cleanup(func() { SetHeirloomMarks(false) })
	h := &Heirlooms{db: db, now: func() time.Time { return now }}
	if got := h.rankOf(102); got != 1 {
		t.Errorf("Heirlooms reads B's rank as %d, want the account's 1", got)
	}
}
