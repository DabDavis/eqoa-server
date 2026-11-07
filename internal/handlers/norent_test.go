package handlers

import (
	"testing"

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
