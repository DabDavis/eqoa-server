package database

import (
	"context"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// A pack's version moves with every write of its rows, through any path, and
// only its own character's: an insert, an update, an update that hands a row
// to another character (both), a delete, a transaction's (at the write and
// again at the commit) and one rolled back; an item record's delete moves
// every character's. A write by a connection without the hooks (another
// process's, as far as this one can tell) moves every version at the next
// NoteOtherWriters, and nothing does when nothing else wrote.
func TestInventoryVersionsFollowTheWrites(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/v.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv := NewInventoryStore(db)
	ver := func(id uint32) uint64 {
		st, ok := InventoryVersion(id)
		if !ok {
			t.Fatal("no versions kept")
		}
		return st.Seq
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	moved := func(what string, f func(), want map[uint32]bool) {
		t.Helper()
		before := map[uint32]uint64{}
		for id := range want {
			before[id] = ver(id)
		}
		f()
		for id, w := range want {
			if got := ver(id) != before[id]; got != w {
				t.Fatalf("%s: character %d's version moved %v, want %v", what, id, got, w)
			}
		}
	}
	exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (1, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Pebble', '', 0, 0)`)
	var pebble int
	db.QueryRow(`SELECT max(patternid) FROM itempattern`).Scan(&pebble)
	row := `INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber) VALUES (?, 1, 0, 0, ?, -1, 0, 0)`
	moved("an insert", func() { exec(row, 77, pebble) }, map[uint32]bool{77: true, 78: false})
	var item int
	db.QueryRow(`SELECT itemid FROM charinventory WHERE serverid = 77`).Scan(&item)
	moved("an update", func() { exec(`UPDATE charinventory SET stackleft = 3 WHERE itemid = ?`, item) }, map[uint32]bool{77: true, 78: false})
	moved("the store's equip", func() {
		if err := inv.SetEquipLoc(77, item, 2); err != nil {
			t.Fatal(err)
		}
	}, map[uint32]bool{77: true, 78: false})
	moved("a hand-over", func() { exec(`UPDATE charinventory SET serverid = 78 WHERE itemid = ?`, item) }, map[uint32]bool{77: true, 78: true, 79: false})
	moved("a delete", func() { exec(`DELETE FROM charinventory WHERE itemid = ?`, item) }, map[uint32]bool{78: true, 77: false})
	// a transaction: at the write, and again at its commit
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	a := ver(79)
	if _, err := tx.Exec(row, 79, pebble); err != nil {
		t.Fatal(err)
	}
	b := ver(79)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if c := ver(79); !(a < b && b < c) {
		t.Fatalf("a transaction's write and commit: versions %d, %d, %d", a, b, c)
	}
	moved("a write rolled back", func() {
		tx, _ := db.Begin()
		tx.Exec(row, 80, pebble)
		tx.Rollback()
	}, map[uint32]bool{80: true, 79: false})
	moved("a quest row", func() { exec(`INSERT INTO charquests (serverid, questid, step) VALUES (81, 1, 1)`) }, map[uint32]bool{81: false})

	// another connection's commit, with no hooks on it: seen at the next ask
	inv.NoteOtherWriters(time.Now())
	moved("nothing else wrote", func() { inv.NoteOtherWriters(time.Now().Add(time.Hour)) }, map[uint32]bool{79: false})
	other, err := NewSQLite(db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	conn, err := other.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Raw(func(dc any) error {
		h := dc.(sqlite.HookRegisterer)
		h.RegisterPreUpdateHook(nil)
		h.RegisterCommitHook(nil)
		return nil
	})
	moved("an unhooked write, before the ask", func() {
		if _, err := conn.ExecContext(ctx, row, 79, pebble); err != nil {
			t.Fatal(err)
		}
	}, map[uint32]bool{79: false})
	moved("the ask after it", func() { inv.NoteOtherWriters(time.Now().Add(2 * time.Hour)) }, map[uint32]bool{79: true, 90: true})
	moved("an ask too soon after", func() {
		conn.ExecContext(ctx, row, 79, pebble)
		inv.NoteOtherWriters(time.Now().Add(2 * time.Hour))
	}, map[uint32]bool{79: false})

	// an item record deleted: every character's
	exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (1, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Gone', '', 0, 0)`)
	moved("an item record deleted", func() { exec(`DELETE FROM itempattern WHERE itemname = 'Gone'`) }, map[uint32]bool{12345: true})
}

// A read taken within InventorySettle of its character's last change is not
// settled; one taken after is.
func TestInventoryStampSettles(t *testing.T) {
	now := time.Now()
	st := InventoryStamp{Seq: 1, At: now}
	if st.Settled(now.Add(InventorySettle / 2)) {
		t.Fatal("a read just after a change settled")
	}
	if !st.Settled(now.Add(InventorySettle)) {
		t.Fatal("a read InventorySettle after a change not settled")
	}
}
