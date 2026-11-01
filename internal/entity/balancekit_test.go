package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A NO RENT item (rent 0: what a spell conjures) is no kit's: a warrior of 30
// at the best tier holds the best weapon it could buy or find, never a
// conjured one that hits harder, and wears no conjured shield. The control:
// the same weapon and shield as ordinary items (rent 1) are its pick. The
// items are made up.
func TestASimKitWearsNothingConjured(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/kit.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent,
		unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6,
		craft, itemname, itemdesc, model, color, AC)
		VALUES (8101, 0, 0, 1, 0, 12, 0, 0, 1, 0, 1, 300, 0, 30, 1, 6000, 50, 32767, 2047, 0, 0, 0, 0, 'Fieldhand Blade', '', 0, 0, 0),
		       (8102, 0, 0, 1, 0, 12, 0, 0, 0, 0, 1, 900, 0, 30, 1, 6000, 50, 32767, 2047, 0, 0, 0, 0, 'Summoned Sabre', '', 0, 0, 0),
		       (8103, 0, 0, 1, 0, 13, 0, 0, 0, 0, 0, 0, 0, 30, 1, 6000, 50, 32767, 2047, 0, 0, 0, 0, 'Summoned Buckler', '', 0, 0, 90)`); err != nil {
		t.Fatal(err)
	}
	pl, err := SimKitFromDB(db, 0, 0, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Gear.WeaponDamage != 300 || pl.Gear.AC != 0 || pl.Worn[17] {
		t.Errorf("kit %+v worn %v: want the 300 blade alone, no conjured sabre or buckler", pl.Gear, pl.Worn)
	}
	if _, err := db.Exec(`UPDATE itempattern SET rent = 1 WHERE patternid IN (8102, 8103)`); err != nil {
		t.Fatal(err)
	}
	pl, err = SimKitFromDB(db, 0, 0, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Gear.WeaponDamage != 900 || pl.Gear.AC != 90 || !pl.Worn[17] {
		t.Errorf("the control: kit %+v worn %v, want the 900 sabre and the 90 buckler", pl.Gear, pl.Worn)
	}
}

// A broken record (a PoT or a resist past any item's: the old dump's 987 PoT,
// 927 LR) is no kit's either; the control: the same sabre with an item's
// figures is the pick. Made-up items.
func TestASimKitWearsNoBrokenRecord(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/kit.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent,
		unk4, attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6,
		craft, itemname, itemdesc, model, color, AC, PoT, LR)
		VALUES (8101, 0, 0, 1, 0, 12, 0, 0, 1, 0, 1, 300, 0, 30, 1, 6000, 50, 32767, 2047, 0, 0, 0, 0, 'Fieldhand Blade', '', 0, 0, 0, 0, 0),
		       (8104, 0, 0, 1, 0, 12, 0, 0, 1, 0, 1, 999, 0, 30, 1, 6000, 50, 32767, 2047, 0, 0, 0, 0, 'Gleaming Sabre', '', 0, 0, 0, 987, 927)`); err != nil {
		t.Fatal(err)
	}
	pl, err := SimKitFromDB(db, 0, 0, 30, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Gear.WeaponDamage != 300 {
		t.Errorf("kit %+v: want the 300 blade, not the broken record's 999", pl.Gear)
	}
	if _, err := db.Exec(`UPDATE itempattern SET PoT = 20, LR = 15 WHERE patternid = 8104`); err != nil {
		t.Fatal(err)
	}
	if pl, err = SimKitFromDB(db, 0, 0, 30, 1); err != nil || pl.Gear.WeaponDamage != 999 {
		t.Errorf("the control: kit %+v (%v), want the sabre's 999", pl.Gear, err)
	}
}
