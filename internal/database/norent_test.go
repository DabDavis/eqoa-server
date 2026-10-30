package database

import (
	"testing"
	"time"
)

// A logout time is kept by the character and read once: the reading takes
// the row away, and a later reading finds none (norent.go).
func TestALogoutTimeIsReadOnce(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/logout.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSQLiteCharacterStore(db)
	at := time.Unix(1_800_000_000, 0)
	if err := s.SaveLogout(7, at.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveLogout(7, at); err != nil { // a later logout replaces it
		t.Fatal(err)
	}
	if got, ok, err := s.TakeLogout(7); err != nil || !ok || !got.Equal(at) {
		t.Fatalf("the logout read as %v (%v, %v), want %v", got, ok, err, at)
	}
	if _, ok, err := s.TakeLogout(7); ok || err != nil {
		t.Errorf("a second reading found one (%v)", err)
	}
	if _, ok, err := s.TakeLogout(8); ok || err != nil {
		t.Errorf("a character with none found one (%v)", err)
	}
}

// A character's No Rent items are those whose record's rent is 0, wherever
// they are; and whether it holds an item counts the bank. Made-up items.
func TestNoRentItemsAndHeldItems(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/norent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := NewInventoryStore(db)
	item := func(name string, rent int) int {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 0, ?, 0, 0, 0, 0, 1, 20, 0, 0, 32767, 1023, 2, 0, 0, 0, ?, '', 0, 0)`, rent, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	drink, sword, stone := item("Made-up Drink", 0), item("Made-up Sword", 1), item("Made-up Stone", 0)
	for _, r := range []struct{ pattern, location int }{{drink, 0}, {sword, 0}, {stone, 1}} {
		if _, err := inv.AddCharItem(9, r.pattern, 1, 0, 0, r.location); err != nil {
			t.Fatal(err)
		}
	}
	items, err := inv.NoRentItems(9)
	if err != nil || len(items) != 2 || items[0].Pattern.PatternID != drink || items[1].Pattern.PatternID != stone {
		t.Fatalf("No Rent items %+v (%v), want the drink and the banked stone", items, err)
	}
	for _, c := range []struct {
		pattern int
		held    bool
	}{{stone, true}, {sword, true}, {item("Made-up Other", 1), false}} {
		if held, err := inv.HoldsPattern(9, c.pattern); err != nil || held != c.held {
			t.Errorf("pattern %d held %v (%v), want %v", c.pattern, held, err, c.held)
		}
	}
}
