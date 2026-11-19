package handlers

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Barrel of Monkeys from the item menu (the owner, 2026-11-16: "give [a player]
// a barrel of monkeys unique item. it spawns a monkey that follows him around,
// up to 5"): each Use lets one more monkey out, named in a line with how many
// are out, up to five; the fifth's line says the next Use sends them home, and
// it does; the barrel is never spent and no pet comes. Bound to its one
// character (itemowner), anyone else's copy is refused in red and lets out
// nothing. The control: another usable item lets out no monkey.
func TestTheBarrelLetsOutMonkeys(t *testing.T) {
	f := newScribeFixture(t)
	old := itemOwners.Load()
	itemOwners.Store(f.db)
	t.Cleanup(func() { itemOwners.Store(old) })
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP, f.player.Level = 100, 100, 60
	f.player.X, f.player.Z = 1000, 1000
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	insert := func(name string) int {
		res, err := f.db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, -1, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 80, 32767, 32767, 2, 1, 0, 0, ?, '', 0, 0)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	toy, barrel := insert("Painted Top"), insert(MonkeyBarrelName)
	if _, err := f.db.Exec(`INSERT INTO itemowner (patternid, serverid, note) VALUES (?, ?, 'test')`, barrel, f.sid); err != nil {
		t.Fatal(err)
	}
	f.own(toy, 1)
	f.own(barrel, 1)
	f.loadList()
	key := func(pattern int) uint32 {
		t.Helper()
		rows, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			if rows[i].Pattern.PatternID == pattern {
				return uint32(i)
			}
		}
		t.Fatalf("no item %d", pattern)
		return 0
	}
	said := func(sent [][]byte, words string) bool {
		for _, m := range sent {
			if (opOf(m) == protocol.OpcodeClientMessage || opOf(m) == protocol.OpcodeColoredChat) && bytes.Contains(m, utf16le(words)) {
				return true
			}
		}
		return false
	}
	monkeys := func() int {
		n := 0
		for _, npc := range w.NPCs {
			if !npc.Dead && npc.Owner == 0 && uint32(npc.ModelID) == entity.MonkeyBody {
				n++
			}
		}
		return n
	}

	f.use(key(toy))
	if monkeys() != 0 {
		t.Fatal("another item let out a monkey")
	}
	for i := 1; i <= entity.MonkeysMax; i++ {
		sent := f.use(key(barrel))
		if monkeys() != i || w.MonkeysOut(f.sess) != i {
			t.Fatalf("use %d: %d monkeys in the world, %d out", i, monkeys(), w.MonkeysOut(f.sess))
		}
		want := fmt.Sprintf("(%d of %d)", i, entity.MonkeysMax)
		if i == entity.MonkeysMax {
			want = "use the barrel again to send them home"
		}
		if !said(sent, "climbs out of the barrel") || !said(sent, want) {
			t.Errorf("use %d's line: %s, want %q", i, ops(sent), want)
		}
	}
	if sent := f.use(key(barrel)); monkeys() != 0 || !said(sent, monkeysBackHome) {
		t.Errorf("a use with five out: %d left, %s", monkeys(), ops(sent))
	}
	if f.stackOf(MonkeyBarrelName) != 1 {
		t.Error("the barrel was spent")
	}
	for _, n := range w.NPCs {
		if n.Owner == f.player.ServerID {
			t.Errorf("a pet came: %q", n.Name)
		}
	}

	// the barrel bound to another character: this one holds a copy (the
	// account bank, a GM's give)
	bind := func(sid int) {
		if _, err := f.db.Exec(`UPDATE itemowner SET serverid = ? WHERE patternid = ?`, sid, barrel); err != nil {
			t.Fatal(err)
		}
	}
	bind(int(f.sid) + 1000)
	if sent := f.use(key(barrel)); monkeys() != 0 || !said(sent, monkeyNotYours) {
		t.Errorf("another's use: %d monkeys, %s", monkeys(), ops(sent))
	}
	bind(int(f.sid))

	// dead, nothing comes
	f.player.HP = 0
	if sent := f.use(key(barrel)); monkeys() != 0 || !said(sent, monkeyDeadLine) {
		t.Errorf("a dead use: %d monkeys, %s", monkeys(), ops(sent))
	}

	if IsMonkeyBarrel(&database.ItemPatternRecord{EquipSlot: 18, ItemName: MonkeyBarrelName}) ||
		!IsMonkeyBarrel(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " barrel of monkeys"}) {
		t.Error("IsMonkeyBarrel")
	}
}
