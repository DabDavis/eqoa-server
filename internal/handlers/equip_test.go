package handlers

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The server must never choose a slot the client's fit check refuses, and must
// offer every slot it accepts: a refused 0x003F is silently ignored by the
// client, which then shows the item unequipped while the database says worn.
func TestEquipTableIsTheClientsFitCheck(t *testing.T) {
	for w := -1; w <= 30; w++ {
		var want []int
		for e := 0; e <= 31; e++ {
			if fitsEquip(w, e) {
				want = append(want, e)
			}
		}
		got := itemSlotToEquipSlot[w]
		if len(want) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("wear %d: table offers %v, fit check accepts %v", w, got, want)
		}
	}
}

// The values that were wrong or missing before, read off func_00CC61A8's jump
// table (0x00CF3E40).
func TestFitCheckCases(t *testing.T) {
	for _, c := range []struct {
		wear, equip int
		ok          bool
	}{
		{16, 18, true}, {16, 14, false}, // bow: ranged only, never primary
		{17, 18, true},                  // thrown
		{20, 14, true}, {20, 15, false}, // fishing
		{21, 15, true}, {21, 14, false}, // bait
		{22, 19, true}, {27, 19, true}, {27, 20, false}, // tools
		{0, 0, false}, {-1, 0, false}, {28, 19, false},
		{2, 21, true}, // robe
	} {
		if got := fitsEquip(c.wear, c.equip); got != c.ok {
			t.Errorf("fitsEquip(%d, %d) = %v, want %v", c.wear, c.equip, got, c.ok)
		}
	}
}

// Msg003F's displacement table (data_00CDA880), wear 6..27, as read off the
// relocated image.
func TestEquipDisplacesTable(t *testing.T) {
	for _, c := range []struct {
		wear, equip int
		want        []int
	}{
		{6, 6, []int{8}},
		{7, 8, []int{7, 6}},
		{12, 14, []int{16, 18, 19}},
		{20, 14, []int{16, 18, 19}},
		{13, 17, []int{15, 16, 18, 20}},
		{14, 14, []int{16, 18, 19}},
		{14, 15, []int{16, 18, 17, 20}},
		{21, 15, []int{16, 18, 17, 20}},
		{15, 16, []int{14, 15, 17, 18, 19, 20}},
		{16, 18, []int{14, 15, 17, 16, 19, 20}},
		{18, 19, []int{16, 18, 14}},
		{18, 20, []int{16, 18, 17, 15}},
		{25, 19, []int{16, 18, 14}},
		{17, 18, nil}, {8, 9, nil}, {11, 13, nil}, {19, 1, nil}, {1, 0, nil},
	} {
		if got := equipDisplaces(c.wear, c.equip); !reflect.DeepEqual(got, c.want) {
			t.Errorf("equipDisplaces(%d, %d) = %v, want %v", c.wear, c.equip, got, c.want)
		}
	}
}

func row(itemID, patternID, wear, equipLoc, location int, name string) database.CharInventoryEntry {
	return database.CharInventoryEntry{
		ItemID: itemID, StackLeft: 1, EquipLoc: equipLoc, Location: location,
		Pattern: database.ItemPatternRecord{PatternID: patternID, EquipSlot: wear, ItemName: name},
	}
}

// A two-hander takes off both hands, the shield, ranged and held — the client
// does exactly that on 0x003F, so the database must too.
func TestEquipTwoHanderDisplacesLikeTheClient(t *testing.T) {
	items := []database.CharInventoryEntry{
		row(101, 1, 12, 14, 0, "Sword"),    // primary, worn
		row(102, 2, 14, 15, 0, "Dagger"),   // secondary, worn
		row(103, 3, 13, 17, 0, "Shield"),   // worn
		row(104, 4, 5, 5, 0, "Tunic"),      // chest, worn: untouched
		row(105, 5, 15, -1, 0, "Greataxe"), // two-hander, in the pack
		row(201, 6, 12, -1, 1, "Banked"),   // bank row: not in the list
	}
	plan, err := planInteractItem(items, newInvList(items[:5]), 4) // the list SendMemoryDump writes: no bank row
	if err != nil {
		t.Fatal(err)
	}
	if !plan.equip || plan.index != 4 || plan.slot != 16 || plan.unequip != -1 {
		t.Fatalf("plan = %+v, want equip index 4 into 16", plan)
	}
	want := []equipChange{{101, -1}, {102, -1}, {103, -1}, {105, 16}}
	if !reflect.DeepEqual(plan.changes, want) {
		t.Fatalf("changes = %v, want %v", plan.changes, want)
	}
}

// A bow goes to the ranged slot (18), not primary, and clears the hands.
func TestEquipBowGoesToRanged(t *testing.T) {
	items := []database.CharInventoryEntry{
		row(1, 1, 12, 14, 0, "Sword"),
		row(2, 2, 16, -1, 0, "Longbow"),
	}
	plan, err := planInteractItem(items, newInvList(items), 1)
	if err != nil {
		t.Fatal(err)
	}
	if plan.slot != 18 || !reflect.DeepEqual(plan.changes, []equipChange{{1, -1}, {2, 18}}) {
		t.Fatalf("plan = %+v", plan)
	}
}

// Fishing poles, bait and crafting tools were refused before.
func TestEquipFishingBaitTools(t *testing.T) {
	for _, c := range []struct{ wear, slot int }{{20, 14}, {21, 15}, {22, 19}, {27, 19}} {
		items := []database.CharInventoryEntry{row(9, 9, c.wear, -1, 0, "x")}
		plan, err := planInteractItem(items, newInvList(items), 0)
		if err != nil || plan.slot != c.slot {
			t.Errorf("wear %d: slot %d err %v, want %d", c.wear, plan.slot, err, c.slot)
		}
	}
}

// A pair takes the free slot; a full pair displaces the first.
func TestEquipPairs(t *testing.T) {
	items := []database.CharInventoryEntry{
		row(1, 1, 8, 9, 0, "Ring A"),
		row(2, 2, 8, -1, 0, "Ring B"),
	}
	plan, _ := planInteractItem(items, newInvList(items), 1)
	if plan.slot != 10 || !reflect.DeepEqual(plan.changes, []equipChange{{2, 10}}) {
		t.Fatalf("free pair slot: %+v", plan)
	}
	items = append(items, row(3, 3, 8, -1, 0, "Ring C"))
	items[1].EquipLoc = 10
	plan, _ = planInteractItem(items, newInvList(items), 2)
	if plan.slot != 9 || !reflect.DeepEqual(plan.changes, []equipChange{{1, -1}, {3, 9}}) {
		t.Fatalf("full pair: %+v", plan)
	}
}

func TestUnequip(t *testing.T) {
	items := []database.CharInventoryEntry{row(7, 7, 1, 0, 0, "Helm")}
	plan, err := planInteractItem(items, newInvList(items), 0)
	if err != nil || plan.equip || plan.unequip != 0 || !reflect.DeepEqual(plan.changes, []equipChange{{7, -1}}) {
		t.Fatalf("plan = %+v err %v", plan, err)
	}
}

// The request carries the KEY. Once a record has left the client's list, the
// rest move down with their keys, so the key no longer equals the position:
// indexing by it would equip the wrong item, and the reply must carry the
// item's NEW list index.
func TestEquipResolvesKeyNotPosition(t *testing.T) {
	items := []database.CharInventoryEntry{
		row(10, 1, 9, -1, 0, "Belt"),
		row(11, 2, 1, -1, 0, "Helm"),
		row(12, 3, 11, -1, 0, "Boots"),
	}
	list := newInvList(items)
	if !list.took(0, 1) { // the Belt leaves (VIItemList::Took)
		t.Fatal("took refused")
	}
	items = items[1:] // and the database row with it

	plan, err := planInteractItem(items, list, 2) // key 2 is the Boots
	if err != nil {
		t.Fatal(err)
	}
	if plan.name != "Boots" || plan.index != 1 || plan.slot != 13 {
		t.Fatalf("plan = %+v, want the Boots at list index 1 into 13", plan)
	}
	if _, err := planInteractItem(items, list, 0); err == nil {
		t.Fatal("key 0 left the list but still resolved")
	}
}

// UseItem writes a RAW byte last (SendOpIntIntByte), not a varint.
func TestParseInteractItem(t *testing.T) {
	key, target, b, ok := parseInteractItem([]byte{0x4d, 0, 0, 0, 0xff, 0xff, 0xff, 0xff, 0x00})
	if !ok || key != 0x4d || target != 0xFFFFFFFF || b != 0 {
		t.Fatalf("live payload: key %d target %#x b %d ok %v", key, target, b, ok)
	}
	_, _, b, ok = parseInteractItem([]byte{1, 0, 0, 0, 2, 0, 0, 0, 0x80})
	if !ok || b != 0x80 {
		t.Fatalf("raw byte 0x80 read as %d ok %v", b, ok)
	}
	if _, _, _, ok := parseInteractItem([]byte{1, 0, 0, 0, 2, 0, 0, 0}); ok {
		t.Fatal("8 bytes accepted")
	}
}

// What the client's dispatcher reads: 0x003F two bytes (index, slot), 0x0040
// one (slot).
func TestEquipReplyBytes(t *testing.T) {
	if got := equipItemMessage(3, 18).Data(); !bytes.Equal(got, []byte{0x3F, 0x00, 0x03, 0x12}) {
		t.Errorf("0x003F = % x", got)
	}
	if got := unequipItemMessage(14).Data(); !bytes.Equal(got, []byte{0x40, 0x00, 0x0E}) {
		t.Errorf("0x0040 = % x", got)
	}
	if protocol.OpcodeEquipItem != 0x003F || protocol.OpcodeUnequipItem != 0x0040 {
		t.Fatal("opcode constants moved")
	}
}

// VIItemList::AddItem: a stackable item stacks onto the first record of the
// same item, clamped; anything else takes a new record under the counter, not
// the key it arrived with; a full list takes nothing.
func TestInvListAdd(t *testing.T) {
	l := newInvList([]database.CharInventoryEntry{
		{ItemID: 1, StackLeft: 15, Pattern: database.ItemPatternRecord{PatternID: 50}},
		{ItemID: 2, StackLeft: 1, Pattern: database.ItemPatternRecord{PatternID: 60}},
	})
	if key, took := l.add(3, 50, 10, 20); key != 0 || took != 5 {
		t.Fatalf("stack onto record 0: key %d took %d, want 0 and 5 (clamped at 20)", key, took)
	}
	if r, _ := l.at(0); r.qty != 20 {
		t.Fatalf("record 0 holds %d", r.qty)
	}
	if key, took := l.add(4, 60, 1, 1); key != 2 || took != 1 {
		t.Fatalf("non-stackable: key %d took %d, want the counter 2", key, took)
	}
	l.took(1, 1) // the record keyed 1 leaves; keys 0 and 2 stay
	if key, _ := l.add(5, 70, 3, 1); key != 3 {
		t.Fatalf("after a removal the next key is %d, want 3 (keys are never reused)", key)
	}
	if i := l.index(3); i != 2 {
		t.Fatalf("key 3 at list index %d, want 2", i)
	}
	for k := 0; k < invListCap; k++ {
		l.add(100+k, 1000+k, 1, 1)
	}
	if len(l.recs) != invListCap {
		t.Fatalf("list holds %d, want the cap %d", len(l.recs), invListCap)
	}
	if _, took := l.add(999, 9999, 1, 1); took != 0 {
		t.Fatal("a full list took a record")
	}
}

// VIItemList::Took: the record goes at none left and the rest move down; a
// slot out of range or holding too little is refused (the client asserts).
func TestInvListTook(t *testing.T) {
	l := newInvList([]database.CharInventoryEntry{
		{ItemID: 1, StackLeft: 3}, {ItemID: 2, StackLeft: 1},
	})
	if !l.took(0, 2) {
		t.Fatal("2 of 3 refused")
	}
	if r, _ := l.at(0); r.qty != 1 || len(l.recs) != 2 {
		t.Fatalf("after 2 of 3: %+v", l.recs)
	}
	if l.took(1, 2) || l.took(5, 1) {
		t.Fatal("an impossible Took was accepted")
	}
	if !l.took(0, 1) || len(l.recs) != 1 || l.recs[0].key != 1 {
		t.Fatalf("record 0 should leave and key 1 move to index 0: %+v", l.recs)
	}
}
