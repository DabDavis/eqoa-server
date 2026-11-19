package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// With the account bank on, a Hardcore character's bank is its own: not the
// account's in the store, not paged, not synced to (petition 2400).
func TestHardcoreKeepsItsOwnBank(t *testing.T) {
	was := entity.HardcoreOn()
	entity.SetHardcoreOn(true)
	t.Cleanup(func() { entity.SetHardcoreOn(was) })
	f := newAcctFixture(t, true, 2000)
	if _, err := f.db.Exec(`UPDATE characters SET hardcore = 1 WHERE serverid = ?`, f.sid["Ash"]); err != nil {
		t.Fatal(err)
	}
	f.own("Birch", f.gem, 3, 5, 1) // the account's bank
	f.own("Ash", f.sword, 1, 1, 1) // Ash's own
	if f.inv.SharesAccountBank(f.sid["Ash"]) || sharesAccountBank(f.sid["Ash"]) {
		t.Error("the Hardcore Ash shares the account bank")
	}
	if !f.inv.SharesAccountBank(f.sid["Birch"]) || !sharesAccountBank(f.sid["Birch"]) {
		t.Error("the control: Birch does not share the account bank")
	}
	if sharedBank(&entity.Player{Name: "Ash", Hardcore: true}) || !sharedBank(&entity.Player{Name: "Birch"}) {
		t.Error("sharedBank by the player")
	}
	ash, err := f.inv.GetBankItems(f.sid["Ash"])
	if err != nil {
		t.Fatal(err)
	}
	if len(ash) != 1 || ash[0].Pattern.PatternID != f.sword {
		t.Errorf("Ash's bank: %d rows, want its own sword alone", len(ash))
	}
	birch, err := f.inv.GetBankItems(f.sid["Birch"])
	if err != nil {
		t.Fatal(err)
	}
	if len(birch) != 3 {
		t.Errorf("Birch's (the account's) bank: %d rows, want 3 (Ash's sword not in it)", len(birch))
	}
}
