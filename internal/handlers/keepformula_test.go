package handlers

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// formulaInputsDB is keepDB with the character holding a sword (attack type
// 1, damage 25) in the main hand and a bow (7, 60) in the ranged slot, and
// three masteries bought (OFF MOD 2 and 3, DEF MOD 4).
func formulaInputsDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	db := keepDB(t)
	inv := database.NewInventoryStore(db)
	for _, it := range []struct {
		name                   string
		slot, attack, dmg, loc int
	}{
		{"Toy Sword", 14, 1, 25, 14},
		{"Toy Bow", 18, 7, 60, equipRanged},
	} {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 1, 0, ?, 0, 1, 1, 0, ?, ?, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, ?, '', 1, 0)`,
			it.slot, it.attack, it.dmg, it.name)
		if err != nil {
			t.Fatal(err)
		}
		pid, _ := res.LastInsertId()
		id, err := inv.AddCharItem(7, int(pid), 1, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.SetEquipLoc(7, id, it.loc); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO cmcatalog (id, mastery, rank, name, parent, cost, level, effects) VALUES
			(901, 'Toy Offence', 1, 'Toy Offence 1', 0, 1, 1, 'OFFMOD:2'),
			(902, 'Toy Offence', 2, 'Toy Offence 2', 901, 1, 1, 'OFFMOD:3'),
			(903, 'Toy Defence', 1, 'Toy Defence 1', 0, 1, 1, 'DEFMOD:4')`,
		`INSERT INTO charcms (serverid, cmid, cost) VALUES (7, 901, 1), (7, 902, 1), (7, 903, 1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// checkFormulaInputs checks the per-character figures the damage formulas
// read that a login and a resume build: the bow's damage and type beside the
// hand's, and the masteries' OFF/DEF points and rank counts.
func checkFormulaInputs(t *testing.T, how string, p *entity.Player) {
	t.Helper()
	g := p.Gear
	if g.RangedDamage != 60 || g.RangedType != 7 || g.WeaponDamage != 25 || g.WeaponType != 1 {
		t.Errorf("%s: gear ranged %d/%d, hand %d/%d; want 60/7 and 25/1", how, g.RangedDamage, g.RangedType, g.WeaponDamage, g.WeaponType)
	}
	if p.CMOffRanks != 2 || p.CMDefRanks != 1 || p.CMEffects[19] != 5 || p.CMEffects[20] != 4 {
		t.Errorf("%s: mastery ranks %d/%d, points %d/%d; want 2/1 and 5/4", how, p.CMOffRanks, p.CMDefRanks, p.CMEffects[19], p.CMEffects[20])
	}
}

// A fresh login builds the formulas' per-character figures from the
// character's items and masteries, and so does a resume through a restart
// from a keep state written before those figures existed (none of them is
// kept: the resume builds the character from its rows as a login does).
func TestFormulaInputsOnLoginAndResume(t *testing.T) {
	db := formulaInputsDB(t)
	_, gw, sm := keepProcess(t, db)
	if _, err := gw.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 7, 7, 7, 1, true)
	sess.SetAccountID(3)
	sess.SetPlayerObjectID(500000007)
	sess.SetState(session.StateInGame)
	sess.SetCharacterInWorld(true)
	rec, err := database.NewSQLiteCharacterStore(db).GetByServerID(7)
	if err != nil {
		t.Fatal(err)
	}
	p := keepChars.Load().enterFromRecord(sess, rec)
	if p == nil {
		t.Fatal("no player")
	}
	checkFormulaInputs(t, "login", p)

	// the keep state as a process before these figures wrote it: its own
	// session half, and a player's and handlers' half of that day's fields
	// alone (no gear, no masteries: they were never kept)
	old := struct {
		S session.Kept
		P json.RawMessage
		E json.RawMessage
	}{S: sess.Keep(), P: json.RawMessage(`{"char":7,"name":"Hero","place":[25330,54.1,15721],"world":0}`), E: json.RawMessage(`{}`)}
	b, _ := json.Marshal(old)

	_, gw2, sm2 := keepProcess(t, db)
	if _, err := gw2.LoadCMCatalog(db); err != nil {
		t.Fatal(err)
	}
	var back struct {
		S session.Kept
		P entity.KeptPlayer
		E KeptExtras
	}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	rs, err := sm2.Resume(back.S)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := ResumeCharacter(rs, back.E, back.P, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if gw2.SessionPlayer(rs) != p2 {
		t.Fatal("the resumed player is not the session's")
	}
	checkFormulaInputs(t, "resume", p2)
}
