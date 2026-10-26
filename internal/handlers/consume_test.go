package handlers

import (
	"bytes"
	"database/sql"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Using a grocer's meal or drink, against the scribe's fixture (a level 23
// druid) with the World's Consume stood in for: the meal or drink it is
// handed, and one taken off the stack by 0x003C as a used scroll is. The
// items' icons are made up.

const (
	mealText  = "A hearty meal that will increase health regeneration."
	drinkText = "A refreshing beverage that increases power regeneration."
)

// ware adds an item pattern worn at equipSlot (-1: nowhere) with its
// description, level and own HoT and PoT (NULL when below 0).
func (f *scribeFixture) ware(name, desc string, equipSlot, level, hot, pot int) int {
	f.t.Helper()
	null := func(v int) sql.NullInt64 { return sql.NullInt64{Int64: int64(v), Valid: v >= 0} }
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color, HoT, PoT)
		VALUES (0, 0, 1511586099, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 50, 0, 50, 32767, 1023, 2, 0, 0, 0, ?, ?, 0, 0, ?, ?)`,
		equipSlot, level, name, desc, null(hot), null(pot))
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// eats stands in for the World's Consume: it records each meal and answers
// err.
func (f *scribeFixture) eats(err error) *[]entity.Consumable {
	var got []entity.Consumable
	f.h.eat = func(_ *session.Session, c entity.Consumable, _ time.Time) error {
		got = append(got, c)
		return err
	}
	return &got
}

func (f *scribeFixture) stackOf(name string) int {
	f.t.Helper()
	for _, r := range f.rowsOf() {
		if r.Pattern.ItemName == name {
			return r.StackLeft
		}
	}
	return 0
}

// A meal with no HoT of its own gives its level; one of a stack of five is
// eaten and leaves by 0x003C (index, sound 0, one); a drink gives its own
// PoT. The last of a stack leaves the list. Each is shown by its item's name
// and a buff's 32x32 icon, never the item's own (the wares here carry the
// 128x64 inventory tile's DictID 1511586099, which a meal must not show).
func TestUseMealEatsOne(t *testing.T) {
	f := newScribeFixture(t)
	got := f.eats(nil)
	bread := f.ware("hearty bread", mealText, -1, 10, -1, -1)
	water := f.ware("well water", drinkText, -1, 2, -1, 7)
	f.own(bread, 5)
	f.own(water, 1)
	f.loadList()

	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem)
	if want := mustHex(t, "3c00000002"); !bytes.Equal(msgs[0], want) {
		t.Errorf("0x003C % X, want % X", msgs[0], want)
	}
	if len(*got) != 1 || (*got)[0] != (entity.Consumable{Name: "hearty bread", Icon: mealEffectIcon, Amount: 10}) {
		t.Fatalf("eaten %+v", *got)
	}
	if n := f.stackOf("hearty bread"); n != 4 {
		t.Errorf("%d bread left, want 4", n)
	}

	msgs = f.use(1)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem)
	if want := mustHex(t, "3c00010002"); !bytes.Equal(msgs[0], want) {
		t.Errorf("0x003C % X, want % X", msgs[0], want)
	}
	if len(*got) != 2 || (*got)[1] != (entity.Consumable{Drink: true, Name: "well water", Icon: drinkEffectIcon, Amount: 7}) {
		t.Fatalf("drunk %+v", *got)
	}
	if n := f.stackOf("well water"); n != 0 || len(f.rowsOf()) != 1 {
		t.Errorf("the last water: %d left, %d rows", n, len(f.rowsOf()))
	}
	if l := sessionInvList(f.sess); l == nil || l.index(1) != -1 {
		t.Error("the last water is still in the client's list mirror")
	}
}

// Refused and kept: a meal above the player's level (Sandstorm's line for
// an item used too low) and one in a fight (ours); nothing is taken.
func TestUseMealRefusals(t *testing.T) {
	f := newScribeFixture(t)
	got := f.eats(entity.ErrConsumeFighting)
	f.own(f.ware("levain", mealText, -1, 30, -1, -1), 3)
	f.own(f.ware("hearty bread", mealText, -1, 10, -1, -1), 3)
	f.loadList()

	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeColoredChat)
	if !bytes.Contains(msgs[0], utf16le("You are not high enough level to equip that item.")) {
		t.Errorf("too low: % X", msgs[0])
	}
	msgs = f.use(1)
	mustOps(t, msgs, protocol.OpcodeColoredChat)
	if !bytes.Contains(msgs[0], utf16le(consumeFighting)) {
		t.Errorf("fighting: % X", msgs[0])
	}
	if len(*got) != 1 || f.stackOf("levain") != 3 || f.stackOf("hearty bread") != 3 {
		t.Errorf("%d meals tried, levain %d, bread %d; want 1, 3, 3", len(*got), f.stackOf("levain"), f.stackOf("hearty bread"))
	}
}

// The controls: an unworn item that is no meal or drink is not eaten, and
// gear worn somewhere is no meal whatever its words.
func TestUseNotAMeal(t *testing.T) {
	f := newScribeFixture(t)
	got := f.eats(nil)
	f.own(f.ware("pebble", "A small grey pebble.", -1, 1, -1, -1), 1)
	f.loadList()
	f.use(0)
	if len(*got) != 0 || f.stackOf("pebble") != 1 {
		t.Errorf("eaten %+v, %d pebbles left", *got, f.stackOf("pebble"))
	}
	hot := 5
	worn := database.ItemPatternRecord{EquipSlot: 6, ItemName: "regen bracers", HoT: &hot,
		ItemDesc: "A hearty meal that will increase health regeneration."}
	if c, ok := consumableOf(&worn); ok {
		t.Errorf("worn bracers taken for a meal: %+v", c)
	}
	worn.EquipSlot = -1
	if _, ok := consumableOf(&worn); !ok {
		t.Error("the same words unworn: no meal")
	}
}

func utf16le(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}
