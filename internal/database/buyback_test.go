package database

import (
	"errors"
	"sync"
	"testing"
)

func buyBackDB(t *testing.T) (*SQLiteDB, *InventoryStore, uint32, *CharacterRecord) {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/buyback.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &CharacterRecord{CharName: "Backer", AccountID: 1}
	if err := NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	return db, NewInventoryStore(db), rec.ServerID, rec
}

func buyBackPattern(t *testing.T, db *SQLiteDB, name string, rent int) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 0, 0, -1, 0, 0, ?, 0, 0, 0, 0, 1, 1, 50, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0)`, rent, name)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

func buyBackRow(pattern, rent, hp, charges int) *CharInventoryEntry {
	return &CharInventoryEntry{RemainHP: hp, Charges: charges, Pattern: ItemPatternRecord{PatternID: pattern, Rent: rent}}
}

// Five kept, newest first; each its own figures. The control: another character's are its own.
// Nothing of the table reads like a secret, which would refuse every snapshot.
func TestBuyBacksAreKeptNewestFirst(t *testing.T) {
	db, s, sid, _ := buyBackDB(t)
	p := buyBackPattern(t, db, "Thing", 1)
	for i := 1; i <= 7; i++ {
		id, kept, err := s.AddBuyBack(sid, buyBackRow(p, 1, i, i+1), i, i*10, BuyBackSold)
		if err != nil || !kept || id == 0 {
			t.Fatalf("add %d: %d %v %v", i, id, kept, err)
		}
	}
	if _, _, err := s.AddBuyBack(sid+1, buyBackRow(p, 1, 1, 0), 1, 1, BuyBackDestroyed); err != nil {
		t.Fatal(err)
	}
	got, err := s.BuyBacks(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != BuyBackKept {
		t.Fatalf("%d kept", len(got))
	}
	for i, e := range got {
		n := 7 - i
		if e.Item.StackLeft != n || e.Item.RemainHP != n || e.Item.Charges != n+1 || e.Price != n*10 || e.How != BuyBackSold || e.Item.Pattern.ItemName != "Thing" || e.Item.EquipLoc != -1 {
			t.Errorf("entry %d: %+v", i, e)
		}
	}
	if other, _ := s.BuyBacks(sid + 1); len(other) != 1 || other[0].How != BuyBackDestroyed {
		t.Errorf("the other character's: %+v", other)
	}
	for _, name := range []string{"charbuyback", "id", "serverid", "patternid", "stack", "remainhp", "charges", "price", "how", "made"} {
		if SecretLikeName(name) {
			t.Errorf("%q reads like a secret: every snapshot would be refused", name)
		}
	}
}

// Taken once: the second finds it gone and gives nothing, however many ask at once.
func TestTakeBuyBackGivesItOnce(t *testing.T) {
	db, s, sid, _ := buyBackDB(t)
	p := buyBackPattern(t, db, "Thing", 1)
	id, _, err := s.AddBuyBack(sid, buyBackRow(p, 1, 37, 3), 1, 10, BuyBackSold)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	took, gone := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.TakeBuyBack(sid, id, 0, 0, 5)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				took++
			case errors.Is(err, ErrBuyBackGone):
				gone++
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if took != 1 || gone != 7 {
		t.Fatalf("took %d, gone %d", took, gone)
	}
	rows, err := s.GetCharacterItems(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RemainHP != 37 || rows[0].Charges != 3 || rows[0].EquipLoc != -1 || rows[0].Location != 0 {
		t.Errorf("the rows: %+v", rows)
	}
	var purse int
	db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, sid).Scan(&purse)
	if purse != 5 {
		t.Errorf("the purse: %d", purse)
	}
}

// What fails leaves everything as it was: a stack that is not there, the entry kept and the purse
// untouched; and an entry is only its own character's to take.
func TestTakeBuyBackIsAllOrNothing(t *testing.T) {
	db, s, sid, _ := buyBackDB(t)
	p := buyBackPattern(t, db, "Thing", 1)
	id, _, err := s.AddBuyBack(sid, buyBackRow(p, 1, 37, 3), 4, 10, BuyBackSold)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TakeBuyBack(sid, id, 99999, 8, 5); err == nil {
		t.Fatal("a stack that is not there took")
	}
	if got, _ := s.BuyBacks(sid); len(got) != 1 {
		t.Errorf("the entry went with the failure: %+v", got)
	}
	var purse int
	db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, sid).Scan(&purse)
	if purse == 5 {
		t.Error("the purse moved with the failure")
	}
	if _, err := s.TakeBuyBack(sid+1, id, 0, 0, 5); !errors.Is(err, ErrBuyBackGone) {
		t.Errorf("another character took it: %v", err)
	}
	// onto a stack of the same record: the row's stack, no new row
	row, err := s.AddCharItem(sid, p, 2, 50, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.TakeBuyBack(sid, id, row, 6, 5); err != nil || got != row {
		t.Fatalf("onto the stack: %d %v", got, err)
	}
	if rows, _ := s.GetCharacterItems(sid); len(rows) != 1 || rows[0].StackLeft != 6 || rows[0].RemainHP != 50 {
		t.Errorf("the stack: %+v", rows)
	}
}

// Deleted records and No Rent drop out quietly, from the table too; No Rent never goes in; a deleted
// character's go with it. The control: the others stay.
func TestBuyBacksThatCannotComeBack(t *testing.T) {
	db, s, sid, rec := buyBackDB(t)
	keep := buyBackPattern(t, db, "Keep", 1)
	deleted := buyBackPattern(t, db, "Deleted", 1)
	madeNoRent := buyBackPattern(t, db, "MadeNoRent", 1)
	vanishes := buyBackPattern(t, db, "Vanishes", 1)
	conjured := buyBackPattern(t, db, "Conjured", 0)
	listed := buyBackPattern(t, db, "Listed", 1)
	if _, err := db.Exec(`INSERT INTO norentvanish (patternid) VALUES (?)`, listed); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ pattern, rent int }{{conjured, 0}, {listed, 1}} {
		if id, kept, err := s.AddBuyBack(sid, buyBackRow(c.pattern, c.rent, 1, 0), 1, 1, BuyBackSold); err != nil || kept || id != 0 {
			t.Errorf("No Rent %d went in: %d %v %v", c.pattern, id, kept, err)
		}
	}
	for _, p := range []int{keep, deleted, madeNoRent, vanishes} {
		if _, kept, err := s.AddBuyBack(sid, buyBackRow(p, 1, 1, 0), 1, 1, BuyBackSold); err != nil || !kept {
			t.Fatalf("add %d: %v %v", p, kept, err)
		}
	}
	db.Exec(`DELETE FROM itempattern WHERE patternid = ?`, deleted)
	db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid = ?`, madeNoRent)
	db.Exec(`INSERT INTO norentvanish (patternid) VALUES (?)`, vanishes)
	got, err := s.BuyBacks(sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Item.Pattern.PatternID != keep {
		t.Errorf("what is left: %+v", got)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM charbuyback WHERE serverid = ?`, sid).Scan(&n)
	if n != 1 {
		t.Errorf("%d rows in the table", n)
	}
	// the sixth's count is of what can come back: dead rows do not push out live ones
	for i := 0; i < 2; i++ {
		s.AddBuyBack(sid, buyBackRow(deletedAgain(t, db), 1, 1, 0), 1, 1, BuyBackSold)
	}
	db.Exec(`DELETE FROM itempattern WHERE itemname = 'Again'`)
	for i := 0; i < 4; i++ {
		s.AddBuyBack(sid, buyBackRow(madeNoRentSafe(t, db), 1, 1, 0), 1, 1, BuyBackSold)
	}
	got, _ = s.BuyBacks(sid)
	if len(got) != BuyBackKept || got[len(got)-1].Item.Pattern.PatternID != keep {
		t.Errorf("dead rows pushed out a live one: %+v", got)
	}

	if err := NewSQLiteCharacterStore(db).Delete(rec.ServerID); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM charbuyback WHERE serverid = ?`, sid).Scan(&n)
	if n != 0 {
		t.Errorf("a deleted character keeps %d", n)
	}
}

func deletedAgain(t *testing.T, db *SQLiteDB) int {
	return buyBackPattern(t, db, "Again", 1)
}

func madeNoRentSafe(t *testing.T, db *SQLiteDB) int {
	return buyBackPattern(t, db, "Filler", 1)
}
