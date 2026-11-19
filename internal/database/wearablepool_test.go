package database

import (
	"fmt"
	"math/rand"
	"testing"
)

// FilterWearable of WearablePool answers exactly what WearablePatterns does,
// record for record and in its order, for every class and a spread of races
// (the game's, the froglok past them, custom ones), levels and slot sets, over
// records of random classes, races, levels and slots, some of them Ascended
// (left out by both).
func TestFilterWearableIsWearablePatterns(t *testing.T) {
	db, _ := creationDB(t)
	r := rand.New(rand.NewSource(1119))
	slots := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 19}
	races := []int{1023, 1024, 32, 991, 1, 512, 2047, 0, 3}
	for i := 0; i < 400; i++ {
		id := 910000 + i
		if _, err := db.Exec(`INSERT INTO itempattern
			(patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (?, 0, 0, 0, 0, ?, 0, 0, 0, 0, 0, 0, 0, ?, 1, 100, 0, ?, ?, 0, 0, 0, 0, ?, '', 0, 0)`,
			id, slots[r.Intn(len(slots))], 1+r.Intn(60), r.Intn(32768), races[r.Intn(len(races))], fmt.Sprint("item ", i)); err != nil {
			t.Fatal(err)
		}
		if i%23 == 0 {
			if _, err := db.Exec(`INSERT INTO itemascended (patternid, base) VALUES (?, ?)`, id, 800000+i); err != nil {
				t.Skipf("no itemascended table to fill here: %v", err)
			}
		}
	}
	inv := NewInventoryStore(db)
	all := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 19}
	pool, err := inv.WearablePool(all)
	if err != nil {
		t.Fatal(err)
	}
	wears := [][]int{{1}, {19}, {5}, {8}, {12, 14}, {15}, {16, 17}, {2}, {13}}
	n := 0
	for class := -1; class <= 15; class++ {
		for _, race := range []int{-1, 0, 2, 5, 9, 10, 11, 12, 20, 26, 31} {
			for _, level := range []int{1, 10, 30, 47, 60} {
				for _, wear := range wears {
					want, err := inv.WearablePatterns(class, race, level, wear)
					if err != nil {
						t.Fatal(err)
					}
					got := FilterWearable(pool, class, race, level, wear)
					if len(got) != len(want) {
						t.Fatalf("class %d race %d level %d wear %v: %d records, want %d", class, race, level, wear, len(got), len(want))
					}
					for i := range want {
						if got[i] != want[i] {
							t.Fatalf("class %d race %d level %d wear %v: record %d is %d, want %d", class, race, level, wear, i, got[i].PatternID, want[i].PatternID)
						}
					}
					n += len(want)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no record matched anything: the test asks nothing")
	}
}
