package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Automatic Snowball Launcher from the item menu (the owner, 2026-11-16:
// "lets give zerkziee an automatic snowball launcher, infinite ammo"): a Use
// switches automatic fire on, told; a second off, told; the launcher is never
// spent. Bound to its one character (itemowner), anyone else's copy is
// refused in red and switches nothing. Dead, it will not start. The control:
// another usable item switches nothing.
func TestTheLauncherToggles(t *testing.T) {
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
	toy, launcher := insert("Painted Top"), insert(SnowLauncherName)
	if _, err := f.db.Exec(`INSERT INTO itemowner (patternid, serverid, note) VALUES (?, ?, 'test')`, launcher, f.sid); err != nil {
		t.Fatal(err)
	}
	f.own(toy, 1)
	f.own(launcher, 1)
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

	f.use(key(toy))
	if w.SnowLauncherOn(f.sess) {
		t.Fatal("another item switched it on")
	}
	if sent := f.use(key(launcher)); !w.SnowLauncherOn(f.sess) || !said(sent, launcherOnLine) {
		t.Fatalf("the first use: on %v, %s", w.SnowLauncherOn(f.sess), ops(sent))
	}
	if sent := f.use(key(launcher)); w.SnowLauncherOn(f.sess) || !said(sent, launcherOffLine) {
		t.Fatalf("the second use: on %v, %s", w.SnowLauncherOn(f.sess), ops(sent))
	}
	if f.stackOf(SnowLauncherName) != 1 {
		t.Error("the launcher was spent")
	}

	// bound to another character: this one holds a copy
	if _, err := f.db.Exec(`UPDATE itemowner SET serverid = ? WHERE patternid = ?`, int(f.sid)+1000, launcher); err != nil {
		t.Fatal(err)
	}
	if sent := f.use(key(launcher)); w.SnowLauncherOn(f.sess) || !said(sent, launcherNotYours) {
		t.Errorf("another's use: on %v, %s", w.SnowLauncherOn(f.sess), ops(sent))
	}
	if _, err := f.db.Exec(`UPDATE itemowner SET serverid = ? WHERE patternid = ?`, f.sid, launcher); err != nil {
		t.Fatal(err)
	}

	// dead, it will not start
	f.player.HP = 0
	if sent := f.use(key(launcher)); w.SnowLauncherOn(f.sess) || !said(sent, launcherDeadLine) {
		t.Errorf("a dead use: on %v, %s", w.SnowLauncherOn(f.sess), ops(sent))
	}

	if IsSnowLauncher(&database.ItemPatternRecord{EquipSlot: 16, ItemName: SnowLauncherName}) ||
		!IsSnowLauncher(&database.ItemPatternRecord{EquipSlot: -1, ItemName: " automatic snowball launcher"}) {
		t.Error("IsSnowLauncher")
	}
}
