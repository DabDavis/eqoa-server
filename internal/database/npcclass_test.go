package database

import "testing"

// npcclass keys a name as the server looks it up: trimmed, any case, no
// trailing '$'.
func TestNPCClasses(t *testing.T) {
	for in, want := range map[string]string{"Crag Wisp": "crag wisp", "crag wisp$": "crag wisp", "Gharial ": "gharial", " Guard Findel ": "guard findel"} {
		if got := NPCClassKey(in); got != want {
			t.Errorf("NPCClassKey(%q) = %q, want %q", in, got, want)
		}
	}
	db, err := NewSQLite(t.TempDir() + "/npcclass.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	got, err := NewNPCStore(db).NPCClasses()
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty table: %v, %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO npcclass (name, class, source) VALUES ('crag wisp', 10, 'sandstorm'), ('guard findel', 3, 'sandstorm')`); err != nil {
		t.Fatal(err)
	}
	got, err = NewNPCStore(db).NPCClasses()
	if err != nil || got["crag wisp"] != 10 || got["guard findel"] != 3 || len(got) != 2 {
		t.Errorf("classes %v, %v", got, err)
	}
}

// The catalog's ranged weapons by model: a model whose every weapon is a bow,
// a crossbow or a thrown one, with its commonest type; armour, a model 0 and
// non-weapons do not count. The control: a model one bow and one sword share
// is not ranged, where any ranged item at all would make it so.
func TestRangedModels(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/ranged.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	items := NewItemPatternStore(db)
	for i, r := range []struct {
		wear, attack int
		model        int64
	}{
		{16, 7, 0x0DA9879A}, {16, 7, 0x0DA9879A}, // two bows
		{16, 9, 0x891F30E3},                      // a crossbow
		{17, 10, int64(int32(-1331049518))},      // a throwing knife, stored signed
		{16, 7, 0x47D981E0}, {14, 1, 0x47D981E0}, // a bow and a sword share it
		{13, 0, 0x343CF977}, // a shield
		{16, 7, 0},          // no model
	} {
		if err := items.UpsertItemPattern(&ItemPatternRecord{PatternID: 900 + i, EquipSlot: r.wear, AttackType: r.attack, Model: r.model, ItemName: "x", ItemDesc: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := NewNPCStore(db).RangedModels()
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint32]byte{0x0DA9879A: 7, 0x891F30E3: 9, 0xB0A9CBD2: 10}
	if len(got) != len(want) {
		t.Errorf("ranged %v, want %v", got, want)
	}
	for m, typ := range want {
		if got[m] != typ {
			t.Errorf("model %08X: %d, want %d", m, got[m], typ)
		}
	}
	if _, ok := got[0x47D981E0]; ok {
		t.Error("the control: a model a sword shares is not a ranged weapon's")
	}
}
