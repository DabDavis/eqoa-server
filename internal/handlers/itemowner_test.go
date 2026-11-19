package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// A record bound to one character (itemowner) is put on by that character
// alone (the owner, 2026-11-16: a player's scale mail, "that he alone can wear
// [a player] only"): another character of the same class, race and level holding
// a copy (as from the account bank) is refused it in the gates' red, and so
// is a merc (no character) and another raid seat's Need (the owner's may); the control: the owner
// puts it on, and an unbound record of the same masks goes on for anyone.
func TestABoundItemIsWornByItsOwnerAlone(t *testing.T) {
	f := newGearFixture(t)
	old := itemOwners.Load()
	itemOwners.Store(f.db)
	t.Cleanup(func() { itemOwners.Store(old) })
	f.player.Class, f.player.Race, f.player.Level = 1, 2, 60 // a dark elf ranger
	bound := f.pattern("Drizzt's Scale Mail Bracers", 7, 0, 0)
	free := f.pattern("Scale Bracers", 7, 0, 0)
	if _, err := f.db.Exec(`UPDATE itempattern SET classuse = 2 WHERE patternid IN (?, ?)`, bound, free); err != nil {
		t.Fatal(err)
	}
	owner := f.sid + 1000
	if _, err := f.db.Exec(`INSERT INTO itemowner (patternid, serverid) VALUES (?, ?)`, bound, owner); err != nil {
		t.Fatal(err)
	}
	f.own(bound, -1)
	f.own(free, -1)
	f.loadList()
	equip := func(key uint32) [][]byte {
		return f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
			w.WriteUint32(key)
			w.WriteUint32(0xFFFFFFFF)
			w.WriteByte(0)
		})
	}
	if out := equip(0); len(out) != 1 || !bytes.Contains(out[0], utf16le(itemNotYours)) {
		t.Fatalf("another ranger's equip of the bound bracers: %x, want %q", out, itemNotYours)
	}
	if w := f.worn(); len(w) != 0 {
		t.Fatalf("the bound bracers were put on: %v", w)
	}
	if out := equip(1); len(out) != 1 || out[0][0] != 0x3F {
		t.Fatalf("the unbound bracers: %x, want the equip", out)
	}

	pat := &database.ItemPatternRecord{PatternID: bound, ClassUse: 2, RaceUse: 1023, LevelReq: 60}
	me := &entity.Player{CharServerID: owner, Class: 1, Race: 2, Level: 60}
	if why := cannotWear(me, pat); why != "" {
		t.Errorf("the owner: %q, want it worn", why)
	}
	if why := cannotWear(mercWearer(database.MercRow{Class: 1, Race: 2}, 60), pat); why != itemNotYours {
		t.Errorf("a merc: %q, want %q", why, itemNotYours)
	}
	if mayNeed(entity.RaidSeat{ServerID: owner + 1, Class: 1, Race: 2, Level: 60}, pat) {
		t.Errorf("another raid seat may Need the bound record")
	}
	if !mayNeed(entity.RaidSeat{ServerID: owner, Class: 1, Race: 2, Level: 60}, pat) {
		t.Errorf("the owner's raid seat may not Need it")
	}
	other := *pat
	other.PatternID = free
	if why := cannotWear(&entity.Player{CharServerID: 7, Class: 1, Race: 2, Level: 60}, &other); why != "" {
		t.Errorf("the unbound record: %q, want it worn", why)
	}
}

// With no database to read (a test's handler) nothing is bound.
func TestNoBindingsWithoutADatabase(t *testing.T) {
	old := itemOwners.Load()
	itemOwners.Store(nil)
	t.Cleanup(func() { itemOwners.Store(old) })
	if _, ok := itemOwnerOf(1); ok {
		t.Fatal("bound with no database")
	}
}
