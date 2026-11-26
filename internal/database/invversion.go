package database

import (
	"context"
	"database/sql"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// invversion.go — a version of each character's pack, so a reader can keep
// what it read until the rows change (the quest marks' passes,
// handlers/questtakecache.go). The owner 2026-11-26: "can we optimize the CPU
// cost"; the marks were 23% of each shard's CPU (pprof, 68 players), a pack
// read (InventoryStore.HandInRows) a player a second among it, almost always
// the rows of the second before.
//
// The rows are written from some thirty places (the inventory's own store,
// trades, mail, the auction, the bank, the GM panel's SQL, ...), so the
// version is not bumped by them but by SQLite: every connection the driver
// opens in this process carries a pre-update hook that bumps the character of
// each charinventory row inserted, updated or deleted (the old row's
// character and the new one's: an item handed over bumps both), at the write
// and again as its transaction commits (a reader that took the version between
// the two read the rows before the commit, and its version is then stale).
// Deleting an item record (itempattern) bumps every character: HandInRows
// joins it. What the hooks cannot see:
//
//   - another process's commit (a data tool's --apply against the live
//     database): NoteOtherWriters asks the connection's PRAGMA data_version,
//     which counts exactly those, and bumps every character when it moved;
//   - the instant between the commit hook and the commit itself: a read
//     begun within InventorySettle of a character's last bump is not to be
//     kept (InventoryStamp.Settled);
//   - a connection opened without the hooks (another driver's): Versions then
//     says so, and a reader reads every time.

// InventorySettle is how long after a character's last bump a read of its
// rows stays provisional: used once, never kept.
const InventorySettle = 2 * time.Second

// inventorySettle is InventorySettle unless a test set another
// (SetInventorySettle).
var inventorySettle atomic.Int64

func init() { inventorySettle.Store(int64(InventorySettle)) }

// SetInventorySettle sets the settle (tests: 0, so a kept read stands on its
// version alone) and returns the one before.
func SetInventorySettle(d time.Duration) time.Duration {
	return time.Duration(inventorySettle.Swap(int64(d)))
}

// InventoryStamp is a character's pack's version: Seq grows at every change
// (its own rows, or every character's), At is when it last did.
type InventoryStamp struct {
	Seq uint64
	At  time.Time
}

// Settled is whether rows read at readAt under this stamp may be kept: read
// at least InventorySettle after the stamp's last bump.
func (s InventoryStamp) Settled(readAt time.Time) bool {
	return readAt.Sub(s.At) >= time.Duration(inventorySettle.Load())
}

var invVersions = struct {
	mu  sync.Mutex
	seq uint64
	all InventoryStamp
	by  map[uint32]InventoryStamp
}{by: map[uint32]InventoryStamp{}}

// invUntracked is set when a connection was opened without the hooks: no
// version can be trusted from then on.
var invUntracked atomic.Bool

// invColumnOK is whether charinventory's second column is serverid in every
// database opened (NewSQLite checks it); false, every write to it bumps every
// character.
var invColumnOK atomic.Bool

func init() {
	invColumnOK.Store(true)
	sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		h, ok := conn.(sqlite.HookRegisterer)
		if !ok {
			invUntracked.Store(true)
			return nil
		}
		hookInventoryWrites(h)
		return nil
	})
}

// hookInventoryWrites puts the hooks on one connection. A connection runs one
// statement at a time, so its pending set needs no lock of its own; the
// versions' do (every connection bumps them).
func hookInventoryWrites(h sqlite.HookRegisterer) {
	var pending []uint32
	pendingAll := false
	h.RegisterPreUpdateHook(func(d sqlite.SQLitePreUpdateData) {
		switch d.TableName {
		case "charinventory":
		case "itempattern":
			if d.Op == sqlite3.SQLITE_DELETE {
				pendingAll = true
				bumpInventory(nil, true)
			}
			return
		default:
			return
		}
		if !invColumnOK.Load() {
			pendingAll = true
			bumpInventory(nil, true)
			return
		}
		// the driver stores each column's value in the slot of the slice it is
		// handed (not through a pointer in it): the slice is passed as is
		var ids []uint32
		vals := make([]any, 2) // itemid, serverid
		owner := func() (uint32, bool) {
			switch v := vals[1].(type) {
			case int64:
				return uint32(v), true
			}
			return 0, false
		}
		if d.Op != sqlite3.SQLITE_INSERT {
			id, ok := uint32(0), d.Old(vals...) == nil
			if ok {
				id, ok = owner()
			}
			if !ok {
				pendingAll = true
				bumpInventory(nil, true)
				return
			}
			ids = append(ids, id)
		}
		if d.Op != sqlite3.SQLITE_DELETE {
			vals[0], vals[1] = nil, nil
			id, ok := uint32(0), d.New(vals...) == nil
			if ok {
				id, ok = owner()
			}
			if !ok {
				pendingAll = true
				bumpInventory(nil, true)
				return
			}
			if len(ids) == 0 || ids[0] != id {
				ids = append(ids, id)
			}
		}
		pending = append(pending, ids...)
		bumpInventory(ids, false)
	})
	h.RegisterCommitHook(func() int32 {
		if pendingAll || len(pending) > 0 {
			bumpInventory(pending, pendingAll)
		}
		pending, pendingAll = pending[:0], false
		return 0 // never vetoes the commit
	})
	h.RegisterRollbackHook(func() {
		// the bumps at the writes stand: a reader reads again, finds the old rows
		pending, pendingAll = pending[:0], false
	})
}

// bumpInventory moves the characters' versions (all: every character's).
func bumpInventory(ids []uint32, all bool) {
	now := time.Now()
	invVersions.mu.Lock()
	defer invVersions.mu.Unlock()
	invVersions.seq++
	st := InventoryStamp{Seq: invVersions.seq, At: now}
	if all {
		invVersions.all = st
		// every character's stamp is older now; the map need not keep them
		clear(invVersions.by)
		return
	}
	for _, id := range ids {
		invVersions.by[id] = st
	}
}

// InventoryVersion is the character's pack's version, false when none is
// kept (a connection was opened without the hooks): every read is then to
// be made again.
func InventoryVersion(serverID uint32) (InventoryStamp, bool) {
	if invUntracked.Load() {
		return InventoryStamp{}, false
	}
	invVersions.mu.Lock()
	defer invVersions.mu.Unlock()
	st := invVersions.all
	if c, ok := invVersions.by[serverID]; ok && c.Seq > st.Seq {
		st = c
	}
	return st, true
}

// checkInventoryColumns notes whether the database's charinventory has
// serverid as its second column, as the hooks read it (initSchema's table).
func (db *SQLiteDB) checkInventoryColumns() {
	rows, err := db.Query(`PRAGMA table_info(charinventory)`)
	if err != nil {
		invColumnOK.Store(false)
		return
	}
	defer rows.Close()
	ok := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			break
		}
		if cid == 1 {
			ok = name == "serverid"
		}
	}
	if !ok {
		log.Printf("Inventory versions: charinventory's second column is not serverid: every write bumps every character")
		invColumnOK.Store(false)
	}
}

// otherWriters is a database's PRAGMA data_version as last asked
// (NoteOtherWriters).
type otherWriters struct {
	mu  sync.Mutex
	at  time.Time
	ver int64
	id  uintptr // the connection it was read on
	// read is whether ver stands (false before the first ask and after a
	// failed one), ever whether one was ever read
	read, ever bool
}

// noteOtherWritersEvery is how often NoteOtherWriters asks at most.
const noteOtherWritersEvery = time.Second

// NoteOtherWriters bumps every character's pack version when a connection
// other than the server's own has committed to the database since the last
// ask (another process's: a data tool; PRAGMA data_version, which a
// connection's own commits never move), at most once a noteOtherWritersEvery.
// The server's pool has one connection (NewSQLite); a value read on another
// connection than the last ask's (the pool made a new one) counts as a change.
func (s *InventoryStore) NoteOtherWriters(now time.Time) {
	if s == nil || s.db == nil {
		return
	}
	w := &s.db.writers
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.read && now.Sub(w.at) < noteOtherWritersEvery {
		return
	}
	w.at = now
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		bumpInventory(nil, true)
		w.read = false
		return
	}
	defer conn.Close()
	var v int64
	if err := conn.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&v); err != nil {
		bumpInventory(nil, true)
		w.read = false
		return
	}
	var id uintptr
	conn.Raw(func(dc any) error {
		id = connIdentity(dc)
		return nil
	})
	// a change since the last ask, or an ask that failed between (what was
	// committed then is unknown)
	if w.read && (v != w.ver || id != w.id) || !w.read && w.ever {
		bumpInventory(nil, true)
	}
	w.ver, w.id, w.read, w.ever = v, id, true, true
}

// connIdentity is a driver connection's identity (its pointer).
func connIdentity(dc any) uintptr {
	v := reflect.ValueOf(dc)
	if v.Kind() == reflect.Pointer {
		return v.Pointer()
	}
	return 0
}
