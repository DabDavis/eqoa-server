package database

import (
	"testing"
	"time"
)

// A duel's stakes round the table: held out of both purses in one go, paid
// to the winner once (a second payment, or one of the wrong sum, is refused),
// a stake of an online player written as the server's figure, an offline one
// added to its row; stakes left held go back to their owners at the next
// start; and a hold whose second write fails leaves the first purse as it was.
func TestDuelStakes(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/duel.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	chars := NewSQLiteCharacterStore(db)
	purse := func(id uint32) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	mk := func(name string, tunar int) uint32 {
		t.Helper()
		rec := &CharacterRecord{CharName: name, AccountID: 1}
		if err := chars.Create(rec); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, tunar, rec.ServerID); err != nil {
			t.Fatal(err)
		}
		return uint32(rec.ServerID)
	}
	a, b := mk("Alpha", 1000), mk("Beta", 500)
	now := time.Unix(1_800_000_000, 0)
	if err := db.DuelHold(7, []DuelStake{{a, 200, 800}, {b, 100, 400}}, now); err != nil {
		t.Fatal(err)
	}
	if purse(a) != 800 || purse(b) != 400 {
		t.Fatalf("held: purses %d and %d", purse(a), purse(b))
	}
	if n, _ := db.DuelHeld(7); n != 300 {
		t.Fatalf("held %d, want 300", n)
	}
	if err := db.DuelHold(7, []DuelStake{{a, 1, 799}}, now); err == nil {
		t.Fatal("a second hold of the same duel's side was taken")
	}
	if purse(a) != 800 {
		t.Fatalf("a refused hold moved the purse to %d", purse(a))
	}
	if err := db.DuelSettle(7, []DuelPay{{ServerID: a, Tunar: 400, Online: true, PurseNow: 1200}}); err == nil {
		t.Fatal("a payment of the wrong sum was taken")
	}
	// Alpha in the world, Beta logged out: Alpha's purse the server's figure
	if err := db.DuelSettle(7, []DuelPay{{ServerID: a, Tunar: 300, Online: true, PurseNow: 1100}, {ServerID: b, Tunar: 0}}); err != nil {
		t.Fatal(err)
	}
	if purse(a) != 1100 || purse(b) != 400 {
		t.Fatalf("paid: purses %d and %d", purse(a), purse(b))
	}
	if err := db.DuelSettle(7, []DuelPay{{ServerID: a, Tunar: 300, Online: true, PurseNow: 1400}}); err == nil || purse(a) != 1100 {
		t.Fatalf("paid twice: %v, purse %d", err, purse(a))
	}
	// an offline winner is paid onto its row
	if err := db.DuelHold(8, []DuelStake{{a, 100, 1000}, {b, 100, 300}}, now); err != nil {
		t.Fatal(err)
	}
	if err := db.DuelSettle(8, []DuelPay{{ServerID: a, Tunar: 0, Online: true, PurseNow: 1000}, {ServerID: b, Tunar: 200}}); err != nil {
		t.Fatal(err)
	}
	if purse(a) != 1000 || purse(b) != 500 {
		t.Fatalf("the offline winner: purses %d and %d", purse(a), purse(b))
	}
	// a stopped server's stakes go back at the next start
	if err := db.DuelHold(9, []DuelStake{{a, 250, 750}, {b, 50, 450}}, now); err != nil {
		t.Fatal(err)
	}
	if n, err := db.DuelRefundHeld(); err != nil || n != 2 {
		t.Fatalf("refunded %d (%v)", n, err)
	}
	if purse(a) != 1000 || purse(b) != 500 {
		t.Fatalf("refunded: purses %d and %d", purse(a), purse(b))
	}
	if n, _ := db.DuelHeld(9); n != 0 {
		t.Fatalf("still held %d", n)
	}
	// a hold naming a character that is not there takes nothing from the other
	if err := db.DuelHold(10, []DuelStake{{a, 100, 900}, {99999, 100, 0}}, now); err == nil {
		t.Fatal("a hold with no such character was taken")
	}
	if purse(a) != 1000 {
		t.Fatalf("a failed hold left Alpha at %d", purse(a))
	}
	if n, _ := db.DuelHeld(10); n != 0 {
		t.Fatalf("a failed hold holds %d", n)
	}
}
