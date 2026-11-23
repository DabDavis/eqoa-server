package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// stackOf is a row's stack, and how many bank rows of a pattern the account holds.
func (f *acctFixture) stackOf(itemID int) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT stackleft FROM charinventory WHERE itemid = ?`, itemID).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *acctFixture) bankRowsOf(pattern int) int {
	f.t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE patternid = ? AND location = 1`, pattern).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

// Petition 2804: a stackable item deposited past the page (the page full)
// joins the account bank's stack of it instead of taking a record of its own,
// up to its stack size; once that stack is full the rest takes a new record.
// The control: a sword (no stack) still takes a record each.
func TestAccountBankDepositPastThePageStacks(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	f.own("Ash", f.sword, 40, 1, database.LocBank)
	banked := f.own("Birch", f.gem, 1, 3, database.LocBank)[0] // past a PS2's page of 40
	gems := f.own("Ash", f.gem, 1, 20, 0)[0]
	swords := f.own("Ash", f.sword, 1, 1, 0)
	f.login("Ash", 40)

	f.bankItem("Ash", false, f.keyOf("Ash", false, gems), 2)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeConfirmBankTunar, protocol.OpcodeColoredChat)
	if n := f.stackOf(banked); n != 5 {
		t.Fatalf("the bank's gems: %d, want 5", n)
	}
	if n := f.bankRowsOf(f.gem); n != 1 {
		t.Fatalf("the bank holds %d records of gems, want 1", n)
	}
	if n := f.stackOf(gems); n != 18 {
		t.Fatalf("the pack's gems: %d, want 18", n)
	}
	// the rest: 15 fill the stack to 20, 3 more take a new record
	f.bankItem("Ash", false, f.keyOf("Ash", false, gems), 18)
	f.sent("Ash")
	if n := f.stackOf(banked); n != 20 {
		t.Fatalf("the bank's first gem stack: %d, want 20", n)
	}
	if n := f.bankRowsOf(f.gem); n != 2 {
		t.Fatalf("the bank holds %d records of gems, want 2", n)
	}
	var total int
	f.db.QueryRow(`SELECT SUM(stackleft) FROM charinventory WHERE patternid = ?`, f.gem).Scan(&total)
	if total != 23 {
		t.Fatalf("%d gems in all, want 23", total)
	}
	if held := sessionInvList(f.sess["Ash"]).held(f.gem); held != 0 {
		t.Fatalf("the pack still holds %d gems", held)
	}
	// the control: a sword takes a record of its own
	before, _ := f.inv.BankRowCount(f.sid["Ash"])
	f.bankItem("Ash", false, f.keyOf("Ash", false, swords[0]), 1)
	f.sent("Ash")
	if after, _ := f.inv.BankRowCount(f.sid["Ash"]); after != before+1 {
		t.Fatalf("the sword: %d records, want %d", after, before+1)
	}
}

// A full page that shows a full stack of the item (the first record of it,
// which the client's AddItem joins) sends the deposit onto the bank's next
// stack of it with room, past the page; Birch's page, showing the full stack,
// is not changed. The control: with no stack with room, a new record.
func TestAccountBankDepositPastAFullStack(t *testing.T) {
	f := newAcctFixture(t, true, 0)
	full := f.own("Birch", f.gem, 1, 20, database.LocBank)[0] // on the page
	f.own("Ash", f.sword, 39, 1, database.LocBank)            // the page is full
	more := f.own("Birch", f.gem, 1, 4, database.LocBank)[0]  // past it
	gems := f.own("Ash", f.gem, 1, 6, 0)[0]
	f.login("Ash", 40)
	f.login("Birch", 40)

	f.bankItem("Ash", false, f.keyOf("Ash", false, gems), 6)
	mustOps(t, f.sent("Ash"), protocol.OpcodeRemoveInvItem, protocol.OpcodeConfirmBankTunar, protocol.OpcodeColoredChat)
	if a, b := f.stackOf(full), f.stackOf(more); a != 20 || b != 10 {
		t.Fatalf("the bank's gem stacks: %d and %d, want 20 and 10", a, b)
	}
	if n := f.bankRowsOf(f.gem); n != 2 {
		t.Fatalf("the bank holds %d records of gems, want 2", n)
	}
	mustOps(t, f.sent("Birch"))

	// the control: both stacks full, the next gems take a record of their own
	f.db.Exec(`UPDATE charinventory SET stackleft = 20 WHERE itemid = ?`, more)
	g := f.own("Ash", f.gem, 1, 3, 0)[0]
	f.login("Ash", 40)
	f.bankItem("Ash", false, f.keyOf("Ash", false, g), 3)
	f.sent("Ash")
	if n := f.bankRowsOf(f.gem); n != 3 {
		t.Fatalf("the bank holds %d records of gems, want 3", n)
	}
}

// bankStackRoom tops stacks up in list order, never past the stack size, and
// none for an item that does not stack.
func TestBankStackRoom(t *testing.T) {
	row := func(id, pat, stack int) database.CharInventoryEntry {
		e := database.CharInventoryEntry{ItemID: id, StackLeft: stack, EquipLoc: -1}
		e.Pattern.PatternID = pat
		e.Pattern.MaxStack = 50
		return e
	}
	rows := []database.CharInventoryEntry{row(1, 7, 50), row(2, 8, 10), row(3, 7, 45), row(4, 7, 40)}
	got := bankStackRoom(rows, 7, 50, 12)
	if len(got) != 2 || got[0] != (bankStackTop{3, 50, 5}) || got[1] != (bankStackTop{4, 47, 7}) {
		t.Fatalf("got %+v", got)
	}
	if got := bankStackRoom(rows, 7, 1, 12); got != nil {
		t.Fatalf("no stack: got %+v", got)
	}
}
