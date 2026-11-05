package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/charmove"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// An approved transfer applies at its character's login (level, items into
// the pack), marked applied with its run, and told once in the world; the
// control: one whose character is still in the world (a login taking it over)
// waits, and a character with none applies nothing.
func TestTransferAppliesAtTheLogin(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (3, 'player', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
			('Idle', 3, 9, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
		`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (6089, 0, 0, 1, 0, 13, 0, 0, 1, 0, 0, 0, 0, 60, 1, 100, 0, 8, 1023, 0, 0, 0, 0, 'Tower of Woe', '', 0, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	approve := func(serverID uint32, name string) database.Transfer {
		t.Helper()
		tr, err := db.FileTransfer(database.Transfer{AccountID: 3, ServerID: serverID, FromName: name, FromLevel: 60}, now)
		if err != nil {
			t.Fatal(err)
		}
		plan, _ := json.Marshal(charmove.Plan{Character: name, CharacterID: serverID, Level: 60, To: "auto",
			Items: []charmove.Item{{Pattern: 6089}}})
		tr, _, err = db.ApproveTransfer(tr.ID, string(plan), "Welcome.", database.GMEdit{Username: "gm"}, now)
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}
	hero := approve(7, "Hero")
	idle := approve(9, "Idle")

	gw := entity.NewWorld()
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), nil, nil, database.NewSpellbookStore(db))
	h.SetGameWorld(gw)
	// Idle is in the world (its client dropped; a login would take it over)
	sm := session.NewManager()
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 9, 9, 9, 1, true)
	if p := gw.CreatePlayer(sess, &database.CharacterRecord{ServerID: 9, CharName: "Idle"}); p == nil {
		t.Fatal("CreatePlayer")
	}
	if h.applyTransfer(9) {
		t.Fatal("applied to a character in the world")
	}
	if !h.applyTransfer(7) {
		t.Fatal("not applied")
	}
	if h.applyTransfer(7) || h.applyTransfer(8) {
		t.Fatal("applied twice, or for a character with none")
	}
	var level, items int
	db.QueryRow(`SELECT level FROM characters WHERE serverid = 7`).Scan(&level)
	db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = 7 AND location = 0 AND patternid = 6089`).Scan(&items)
	if level != 60 || items != 1 {
		t.Fatalf("level %d, %d in the pack", level, items)
	}
	got, _, _ := db.TransferByID(hero.ID)
	if got.Status != database.TransferApplied || got.Run == "" {
		t.Fatalf("hero's transfer %+v", got)
	}
	if got, _, _ := db.TransferByID(idle.ID); got.Status != database.TransferApproved {
		t.Fatalf("idle's transfer %s", got.Status)
	}
	// told once in the world: its arrival with the GM's word; Idle's, still
	// approved, as ready on Hero (another character of the account)
	hs := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 5007}, 7, 7, 7, 1, true)
	hs.SetAccountID(3)
	hp := gw.CreatePlayer(hs, &database.CharacterRecord{ServerID: 7, CharName: "Hero"})
	tellTransfers(db, hs, hp)
	lines := gmReplies(t, hs)
	if len(lines) != 4 || !strings.Contains(lines[0], "has arrived (transfer #") || lines[1] != "A GM says: Welcome." ||
		!strings.Contains(lines[2], fmt.Sprintf("transfer #%d is ready: log Idle in to receive it", idle.ID)) {
		t.Fatalf("the lines at the first entry: %q", lines)
	}
	tellTransfers(db, hs, hp)
	if lines := gmReplies(t, hs); len(lines) != 0 {
		t.Fatalf("told again: %q", lines)
	}
}

// An approval is told at once on a character of the account in the world
// (the transfer's own: log out and in), with the items approximated and the
// ask to report them, and marked told; none in the world, not told, so the
// next entry tells it.
func TestTransferApprovedIsTold(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO accountinfo (accountid, username, pwhash, acctstatus, acctlevel) VALUES (3, 'player', 'x', 1, 0)`,
		`INSERT INTO characters (charName, accountid, serverid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle,
			faceoption, sex, world, x, y, z) VALUES ('Hero', 3, 7, 1, 3, 5, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	tr, err := db.FileTransfer(database.Transfer{AccountID: 3, ServerID: 7, FromName: "Hero", FromLevel: 60}, now)
	if err != nil {
		t.Fatal(err)
	}
	plan := `{"character": "Hero", "serverid": 7, "new_records": [{"ref": "new1", "like": 1, "set": {"itemname": "No-Dachi of Toxicity"}},
		{"ref": "new2", "like": 1, "set": {"itemname": "Frosted Tattered Cape"}}], "items": [{"new": "new1"}, {"new": "new2"}]}`
	if tr, _, err = db.ApproveTransfer(tr.ID, plan, "", database.GMEdit{Username: "gm"}, now); err != nil {
		t.Fatal(err)
	}
	gw := entity.NewWorld()
	sm := session.NewManager()
	if TellTransferApproved(db, gw, sm, tr) {
		t.Fatal("told with no one in the world")
	}
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 5007}, 7, 7, 7, 1, true)
	sess.SetAccountID(3)
	sess.SetCharacterInWorld(true)
	gw.CreatePlayer(sess, &database.CharacterRecord{ServerID: 7, CharName: "Hero"})
	if !TellTransferApproved(db, gw, sm, tr) {
		t.Fatal("not told on the transfer's own character")
	}
	lines := gmReplies(t, sess)
	all := strings.Join(lines, " ")
	if len(lines) < 2 || !strings.Contains(lines[0], "is ready: log out and back in to receive it") ||
		!strings.Contains(all, "approximated: No-Dachi of Toxicity, Frosted Tattered Cape.") ||
		!strings.Contains(all, "correct stats, icon and model") {
		t.Fatalf("lines %q", lines)
	}
	for _, l := range lines {
		if len([]rune(l)) > transferLineMax {
			t.Errorf("a line of %d characters: %q", len([]rune(l)), l)
		}
	}
	if left, _ := db.TransfersReadyToTell(3); len(left) != 0 {
		t.Fatal("told but not marked")
	}
}
