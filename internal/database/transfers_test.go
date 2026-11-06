package database

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// transferDB is a database with two accounts, a character each (Hero is
// account 3's), and a few item records.
func transferDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (3, 'player', 'x', 1, 0), (4, 'other', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Else', 4, 8, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (1035, 0, 0, 1, 0, 3, 0, 0, 1, 0, 0, 0, 0, 59, 1, 100, 0, 13, 1023, 0, 0, 0, 0, 'slayer''s claw earring', '', 0, 0),
			       (7648, 0, 0, 1, 0, 3, 0, 0, 1, 0, 0, 0, 0, 59, 1, 100, 0, 0, 1023, 0, 0, 0, 0, 'Slayer Claw Earring', '', 0, 0),
			       (8016, 0, 0, 1, 0, 4, 0, 0, 1, 0, 0, 0, 0, 57, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, 'plated collar of the distant min', '', 0, 0),
			       (5408, 0, 0, 1, 0, 15, 0, 0, 1, 0, 2, 789, 0, 60, 1, 100, 0, 12, 1023, 0, 0, 0, 0, 'naginata of toxicity', '', 0, 0),
			       (6089, 0, 0, 1, 0, 13, 0, 0, 1, 0, 0, 0, 0, 60, 1, 100, 0, 8, 1023, 0, 0, 0, 0, 'Tower of Woe', '', 0, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	return db
}

// A gear list's lines: a count after or before the name, a note in brackets
// and a bullet dropped; no name, no line.
func TestParseItemLine(t *testing.T) {
	for _, c := range []struct {
		in    string
		name  string
		count int
	}{
		{"Slayers claw earring x2", "Slayers claw earring", 2},
		{"Strongest corroded plate breastplate (was that a SS only thing?)", "Strongest corroded plate breastplate", 1},
		{"- 2x Tower of woe", "Tower of woe", 2},
		{"Ring of exploration (2)", "Ring of exploration", 2},
		{"  Frosted tattered cape  ", "Frosted tattered cape", 1},
		{"1. No-dachi of toxicity x 3 (no idea)", "No-dachi of toxicity", 3},
	} {
		name, count, ok := ParseItemLine(c.in)
		if !ok || name != c.name || count != c.count {
			t.Errorf("%q: %q x%d %v", c.in, name, count, ok)
		}
	}
	if _, _, ok := ParseItemLine("  (nothing) "); ok {
		t.Error("a line with no name")
	}
}

// Lines matched to records: an apostrophe and a plural aside ("Slayers claw
// earring" is slayer's claw earring, and the no-apostrophe record is a
// candidate after), our 32-character cut, words in common, and nothing for a
// name this server lacks.
func TestMatchItemLines(t *testing.T) {
	db := transferDB(t)
	lines, err := db.MatchItemLines("Slayers claw earring x2\nPlates collar of the distant mind\nPlated collar of the distant mind\nNo-dachi of toxicity\nFrosted tattered cape\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Fatalf("%d lines", len(lines))
	}
	first := func(l ItemLine) int {
		if len(l.Candidates) == 0 {
			return 0
		}
		return l.Candidates[0].PatternID
	}
	if !lines[0].Exact || first(lines[0]) != 1035 || lines[0].Count != 2 {
		t.Errorf("earring: %+v", lines[0])
	}
	if lines[1].Exact || first(lines[1]) != 8016 {
		t.Errorf("Plates collar (words in common): %+v", lines[1])
	}
	if !lines[2].Exact || first(lines[2]) != 8016 {
		t.Errorf("the 32-character cut: %+v", lines[2])
	}
	if lines[3].Exact || first(lines[3]) != 5408 {
		t.Errorf("No-dachi (the naginata of toxicity has words in common): %+v", lines[3])
	}
	if lines[4].Exact || len(lines[4].Candidates) != 0 {
		t.Errorf("a name this server lacks: %+v", lines[4])
	}
}

// A transfer filed, listed, cancelled; a second open one for the same
// character refused, another account's character refused; approved (audited,
// the plan kept), found as the character's approved one, applied, told.
func TestTransferLife(t *testing.T) {
	db := transferDB(t)
	now := time.Date(2026, 11, 5, 5, 0, 0, 0, time.UTC)
	filed, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromClass: "Shadow Knight", FromLevel: 60,
		FromMasteries: 1040, Items: "Tower of woe\r\nSlayers claw earring x2", Notes: "quests\x07 later"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if filed.Status != TransferWaiting || filed.CharName != "Hero" || filed.FromServer != "Sandstorm" ||
		filed.Items != "Tower of woe\nSlayers claw earring x2" || filed.Notes != "quests later" {
		t.Fatalf("filed %+v", filed)
	}
	if _, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60}, now); err != ErrTransferOpen {
		t.Fatalf("a second open one: %v", err)
	}
	if _, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 8, FromName: "Else", FromLevel: 60}, now); err == nil ||
		!strings.Contains(err.Error(), "not one of your characters") {
		t.Fatalf("another account's character: %v", err)
	}
	if _, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "X", FromLevel: 61}, now); err == nil {
		t.Fatal("level 61")
	}
	if err := db.CancelTransfer(4, filed.ID, now); err != ErrTransferNotFound {
		t.Fatalf("another account cancels: %v", err)
	}
	if err := db.CancelTransfer(3, filed.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.CancelTransfer(3, filed.ID, now); err != ErrTransferDecided {
		t.Fatalf("cancelled twice: %v", err)
	}
	again, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60}, now)
	if err != nil {
		t.Fatalf("filed again after a cancel: %v", err)
	}
	gm := GMEdit{Username: "gm", Via: "test"}
	if _, _, err := db.RejectTransfer(again.ID, " ", gm, now); err == nil {
		t.Fatal("a rejection with no word for the player")
	}
	approved, audit, err := db.ApproveTransfer(again.ID, `{"serverid":7,"level":60}`, "Welcome over.", gm, now)
	if err != nil || audit == 0 || approved.Status != TransferApproved || approved.Plan == "" || approved.DecidedBy == "" {
		t.Fatalf("approved %+v audit %d %v", approved, audit, err)
	}
	got, ok, err := db.ApprovedTransfer(7)
	if err != nil || !ok || got.ID != again.ID {
		t.Fatalf("the character's approved one: %+v %v %v", got, ok, err)
	}
	if err := db.MarkTransferApplied(again.ID, "run-1", now); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkTransferApplied(again.ID, "run-1", now); err == nil {
		t.Fatal("applied twice")
	}
	tell, _ := db.TransfersToTell(7)
	if len(tell) != 1 {
		t.Fatalf("to tell: %d", len(tell))
	}
	db.MarkTransferTold(again.ID)
	if tell, _ := db.TransfersToTell(7); len(tell) != 0 {
		t.Fatal("told twice")
	}
	l, err := db.Transfers(TransferQuery{Status: "all"})
	if err != nil || l.Matching != 2 || l.Counts[TransferApplied] != 1 || l.Counts[TransferCancelled] != 1 {
		t.Fatalf("list %+v %v", l, err)
	}
	if mine, _ := db.AccountTransfers(3); len(mine) != 2 || mine[0].ID != again.ID {
		t.Fatalf("the account's %d", len(mine))
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'chartransfers'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d audit lines", n)
	}
}

// An account may have a transfer open for every character it has, however
// many (the user, 2026-11-06: "it is capped at 3, lets unlimit it"), one a
// character still: the control, a second for the same character, is refused.
func TestTransfersOpenForAnyNumberOfCharacters(t *testing.T) {
	db := transferDB(t)
	for i := 0; i < 6; i++ {
		if _, err := db.Exec(`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor,
			hairlength, hairstyle, faceoption, sex, world, x, y, z) VALUES (?, 3, ?, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
			fmt.Sprintf("Alt%c", 'a'+i), 20+i); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	for i := 0; i < 6; i++ {
		tr, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: uint32(20 + i), FromName: "Alt", FromLevel: 60}, now)
		if err != nil {
			t.Fatalf("transfer %d: %v", i+1, err)
		}
		if i%2 == 0 {
			if _, _, err := db.ApproveTransfer(tr.ID, `{"character": "Alt"}`, "", GMEdit{Username: "gm"}, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.FileTransfer(Transfer{AccountID: 3, ServerID: 20, FromName: "Alt", FromLevel: 60}, now); err != ErrTransferOpen {
		t.Fatalf("a second for the same character: %v", err)
	}
}
