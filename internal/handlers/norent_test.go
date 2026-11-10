package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A record named in norentvanish goes from its holder at the next login, from the pack, the bank and what it
// wears, and only that record: a NO RENT record not named stays, as does another's copy until its own login.
// A character still in the world keeps it: it has not left.
func TestNoRentVanishesAtTheLogin(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/norent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := func(id, rent int, name string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color,
			str, sta, agi, wis, dex, cha, intelligence, HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR, weaponproc, fish)
			VALUES (?, 0, 0, 1, 0, 13, 0, 0, ?, 0, 0, 0, 0, 60, 1, 100, 0, 8, 1023, 0, 0, 0, 0, ?, '', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)`, id, rent, name); err != nil {
			t.Fatal(err)
		}
	}
	item(1, 0, "Belt of the Dragon")
	item(2, 0, "Rogue's Dagger")
	item(3, 1, "Belt of the Dragon")
	if _, err := db.Exec(`INSERT INTO norentvanish (patternid, note) VALUES (1, 'test')`); err != nil {
		t.Fatal(err)
	}
	row := func(id, serverID, pattern, location, equiploc int) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, ?, 1, 100, 0, ?, ?, ?, ?)`, id, serverID, pattern, equiploc, location, id); err != nil {
			t.Fatal(err)
		}
	}
	row(10, 7, 1, 0, -1) // in the pack
	row(11, 7, 1, 1, -1) // in the bank
	row(12, 7, 1, 0, 11) // worn
	row(13, 7, 2, 0, -1) // NO RENT, not named
	row(14, 7, 3, 0, -1) // the original, of the same name
	row(15, 8, 1, 0, -1) // another's copy
	count := func(serverID, pattern int) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, serverID, pattern).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), nil, nil, database.NewSpellbookStore(db))
	h.vanishNoRent(7)
	if got := count(7, 1); got != 0 {
		t.Errorf("%d copies left after the login, want none", got)
	}
	if count(7, 2) != 1 || count(7, 3) != 1 {
		t.Errorf("a record not named went: dagger %d, original %d", count(7, 2), count(7, 3))
	}
	if count(8, 1) != 1 {
		t.Error("another character's copy went at this one's login")
	}
	vanishedMu.Lock()
	told := len(vanished[7])
	delete(vanished, 7)
	vanishedMu.Unlock()
	if told != 3 {
		t.Errorf("%d names kept to tell, want 3", told)
	}

	// still in the world: it keeps it
	gw := entity.NewWorld()
	h.SetGameWorld(gw)
	gw.Players[900] = &entity.Player{ServerID: 900, CharServerID: 8, Name: "Live"}
	if _, live := gw.PlayerByCharacter(8); !live {
		t.Skip("the world does not find the player by character here")
	}
	h.vanishNoRent(8)
	if count(8, 1) != 1 {
		t.Error("a character still in the world lost its copy")
	}
}

// A login within NoRentAway of the character's last logout keeps its No Rent items (jib's PR #1, the GM
// Jibrish's ruling of 2026-10-29: No Rent "means it vanishes if the player logs out for more than 5 minutes"),
// and the logout row is read once. The controls: a login six minutes after it, and one with no row at all (a
// crash, the first login after this came), take them as before.
func TestNoRentStaysWithinFiveMinutesAway(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/norentaway.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color,
		str, sta, agi, wis, dex, cha, intelligence, HPMAX, POWMAX, PoT, HoT, AC, PR, DR, FR, CR, LR, AR, weaponproc, fish)
		VALUES (1, 0, 0, 1, 0, 13, 0, 0, 0, 0, 0, 0, 0, 60, 1, 100, 0, 8, 1023, 0, 0, 0, 0, 'Made-up Belt', '', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO norentvanish (patternid, note) VALUES (1, 'test')`); err != nil {
		t.Fatal(err)
	}
	give := func(serverID int) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO charinventory (itemid, serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, ?, 1, 100, 0, 1, -1, 0, 1)`, 100+serverID, serverID); err != nil {
			t.Fatal(err)
		}
	}
	held := func(serverID int) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND patternid = 1`, serverID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	now := time.Unix(1_800_000_000, 0)
	defer func(f func() time.Time) { noRentNow = f }(noRentNow)
	noRentNow = func() time.Time { return now }
	h := NewCharacterHandler(database.NewSQLiteCharacterStore(db), nil, nil, database.NewSpellbookStore(db))
	defer func() {
		vanishedMu.Lock()
		delete(vanished, 21)
		delete(vanished, 22)
		delete(vanished, 23)
		vanishedMu.Unlock()
	}()

	give(21)
	if err := db.SaveLogout(21, now.Add(-4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	h.vanishNoRent(21)
	if !held(21) {
		t.Error("a login four minutes after the logout took the item")
	}
	if _, ok, _ := db.TakeLogout(21); ok {
		t.Error("the logout's row was not taken away by the login")
	}

	give(22)
	if err := db.SaveLogout(22, now.Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	h.vanishNoRent(22)
	if held(22) {
		t.Error("the control: a login six minutes after the logout kept the item")
	}

	give(23)
	h.vanishNoRent(23)
	if held(23) {
		t.Error("the control: a login with no logout kept the item")
	}
}
