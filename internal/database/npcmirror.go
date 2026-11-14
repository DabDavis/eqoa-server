package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// npcmirror.go — a GM's NPC row edits on one shard written to another shard's
// database as well (the owner, 2026-11-14: "./copy should work for jib also on
// live server, also his placements and copies should show on heirloom also";
// handlers/npcmirror.go says whose and which commands). Both shards' servers run
// on one machine, so the shard that mirrors (the Live Server) opens the other's
// file (Heirloom's, world.mirror_npc_db) for each edit and closes it again.
//
//   - a new row (/place, /clone, /copy): a new npcs row in the other database,
//     of its own id sequence. When the row it was copied from (its source) is a
//     row of the same name there too, the new row is a copy of THAT row (the
//     other shard's own version of the creature), with every column in which
//     this row differs from its source here (its spot, facing and world; a level
//     an edit gave it since) set as here; else this row's values. Its extras
//     (npcRowExtras that are copied: respawn, wander, title, sign text, stands)
//     are made as this row's are.
//   - a later edit of a mirrored row (/move, /roamer, /size): the named columns
//     and the extras made as this row's are now.
//   - a /delete of a mirrored row: its copy deleted with its extras.
//
// Every change has its gm_audit line in the other database ("mirrored from
// Live: " + the command's own detail), in one transaction with table npc_mirror
// (this row's id, the copy's id) and a line of table npc_mirror_inbox, which the
// other shard's server reads every few seconds to put the row into its running
// world (TakeNPCMirrorInbox; handlers' watchNPCMirrorInbox).

// npcMirrorSchema is the mapping and the inbox, in the database written to.
const npcMirrorSchema = `
CREATE TABLE IF NOT EXISTS npc_mirror (
	from_id INTEGER PRIMARY KEY,
	to_id   INTEGER NOT NULL UNIQUE,
	account TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS npc_mirror_inbox (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	npc_row INTEGER NOT NULL,
	gone    INTEGER NOT NULL DEFAULT 0,
	tables  TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL
);`

// NPCMirrorPrefix begins the detail of every audit line a mirror writes.
const NPCMirrorPrefix = "mirrored from Live: "

// npcMirrorBusyMS is how long a mirror's statement waits for the other server's
// lock: short, as the GM's command waits on it.
const npcMirrorBusyMS = 2000

// OpenNPCMirrorPeer opens another shard's database for a mirror's write: an
// existing file only (a wrong path must not make an empty database), with a
// short busy timeout and none of this server's schema run on it. It must have
// an npcs table. The caller closes it.
func OpenNPCMirrorPeer(path string) (*SQLiteDB, error) {
	if path == "" {
		return nil, errors.New("no database named (world.mirror_npc_db)")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=rw&_pragma=busy_timeout(%d)", path, npcMirrorBusyMS))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	peer := &SQLiteDB{DB: db, path: path}
	if !tableExists(peer, "npcs") {
		db.Close()
		return nil, fmt.Errorf("%s has no npcs table", path)
	}
	return peer, nil
}

// SameFile reports whether db is the database file at path.
func (db *SQLiteDB) SameFile(path string) bool {
	a, err1 := os.Stat(db.path)
	b, err2 := os.Stat(path)
	return err1 == nil && err2 == nil && os.SameFile(a, b)
}

// The kinds of a mirrored edit.
const (
	MirrorCreate = "create"
	MirrorUpdate = "update"
	MirrorDelete = "delete"
)

// NPCMirrorEdit is one row edit to mirror.
type NPCMirrorEdit struct {
	Kind   string // MirrorCreate, MirrorUpdate or MirrorDelete
	FromID int64  // the row here
	Source int64  // MirrorCreate: the row here it was copied from (0: none)
	Cols   []string
	Detail string // the command's own audit detail ("/place in the game: ...")
	Who    GMEdit // the GM, by its account id (the shards share their accounts)
}

// NPCMirrored is what a mirror did.
type NPCMirrored struct {
	ToID   int64    // the row in the other database
	Mapped bool     // false: an update or delete of a row that was never mirrored (nothing done)
	Tables []string // the tables changed there, npcs first
}

// npcMirrorCopied are npcRowExtras's tables a mirror makes as the row's.
func npcMirrorCopied() []struct{ Table, Key string } {
	var out []struct{ Table, Key string }
	for _, x := range npcRowExtras {
		if x.Copy {
			out = append(out, struct{ Table, Key string }{x.Table, x.Key})
		}
	}
	return out
}

// rowOf is fullRow outside a transaction.
func (db *SQLiteDB) rowOf(query string, args ...any) (map[string]any, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return fullRow(tx, query, args...)
}

// sameValue is whether two values as fullRow reads them are equal.
func sameValue(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// MirrorNPCEdit writes e, an edit of npcs row e.FromID of db, to peer, in one
// transaction (the file's doc). An update or a delete of a row never mirrored
// does nothing (Mapped false); a create of a row mirrored already is an update
// of every column.
func (db *SQLiteDB) MirrorNPCEdit(peer *SQLiteDB, e NPCMirrorEdit) (NPCMirrored, error) {
	var m NPCMirrored
	if _, err := peer.Exec(npcMirrorSchema); err != nil {
		return m, err
	}
	if _, err := peer.Exec(GMAuditSchema); err != nil {
		return m, err
	}
	// this side's rows first: none of this database is held while the other is written
	var row, src map[string]any
	extras := map[string]map[string]any{}
	if e.Kind != MirrorDelete {
		var err error
		if row, err = db.rowOf(`SELECT * FROM npcs WHERE id = ?`, e.FromID); err != nil {
			return m, err
		}
		if row == nil {
			return m, fmt.Errorf("row %d is not here any more", e.FromID)
		}
		if e.Source > 0 {
			if src, err = db.rowOf(`SELECT * FROM npcs WHERE id = ?`, e.Source); err != nil {
				return m, err
			}
		}
		for _, x := range npcMirrorCopied() {
			if !tableExists(db, x.Table) {
				continue
			}
			r, err := db.rowOf(`SELECT * FROM `+x.Table+` WHERE `+quoteCol(x.Key)+` = ?`, e.FromID)
			if err != nil {
				return m, err
			}
			extras[x.Table] = r
		}
	}

	tx, err := peer.Begin()
	if err != nil {
		return m, err
	}
	defer tx.Rollback()
	who := e.Who
	fillWho(tx, &who)
	detail := NPCMirrorPrefix + e.Detail
	var to int64
	switch err := tx.QueryRow(`SELECT to_id FROM npc_mirror WHERE from_id = ?`, e.FromID).Scan(&to); {
	case errors.Is(err, sql.ErrNoRows):
		to = 0
	case err != nil:
		return m, err
	}
	kind := e.Kind
	cols := e.Cols
	if kind == MirrorCreate && to != 0 {
		kind, cols = MirrorUpdate, nil
		for c := range row {
			if c != "id" {
				cols = append(cols, c)
			}
		}
		sort.Strings(cols)
	}
	if kind != MirrorCreate && to == 0 {
		return m, nil
	}
	m.Mapped, m.ToID = true, to
	gone := 0

	switch kind {
	case MirrorDelete:
		uses, err := npcRowUses(tx, to)
		if err != nil {
			return m, err
		}
		if len(uses) > 0 {
			return m, &ErrNPCRowInUse{Uses: uses}
		}
		for _, x := range npcRowExtras {
			if !tableExists(tx, x.Table) {
				continue
			}
			did, err := auditedDelete(tx, who, x.Table, x.Key, to, detail)
			if err != nil {
				return m, fmt.Errorf("%s: %w", x.Table, err)
			}
			if did {
				m.Tables = append(m.Tables, x.Table)
			}
		}
		did, err := auditedDelete(tx, who, "npcs", "id", to, detail)
		if err != nil {
			return m, err
		}
		if did {
			m.Tables = append([]string{"npcs"}, m.Tables...)
		}
		if _, err := tx.Exec(`DELETE FROM npc_mirror WHERE from_id = ?`, e.FromID); err != nil {
			return m, err
		}
		gone = 1

	case MirrorCreate:
		have, err := tableColumns(tx, "npcs")
		if err != nil {
			return m, err
		}
		values := row
		if src != nil {
			theirs, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ? AND npc_name = ? COLLATE NOCASE`, e.Source, row["npc_name"])
			if err != nil {
				return m, err
			}
			if theirs != nil {
				values = theirs
				for c, v := range row {
					if _, ok := theirs[c]; ok && c != "id" && !sameValue(v, src[c]) {
						values[c] = v
					}
				}
			}
		}
		id, err := insertRowValues(tx, "npcs", have, values, "id")
		if err != nil {
			return m, err
		}
		to, m.ToID = id, id
		after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
		if err != nil {
			return m, err
		}
		if _, err := auditRowChange(tx, who, "insert", "npcs", map[string]any{"id": id}, nil, after, detail); err != nil {
			return m, err
		}
		m.Tables = append(m.Tables, "npcs")
		if _, err := tx.Exec(`INSERT INTO npc_mirror (from_id, to_id, account, at) VALUES (?, ?, ?, ?)`,
			e.FromID, id, who.Username, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return m, err
		}

	case MirrorUpdate:
		before, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, to)
		if err != nil {
			return m, err
		}
		if before == nil {
			return m, fmt.Errorf("its copy there (row %d) is gone", to)
		}
		var set []string
		var args []any
		for _, c := range cols {
			v, ok := row[c]
			if _, theirs := before[c]; !ok || !theirs || c == "id" || sameValue(v, before[c]) {
				continue
			}
			set = append(set, quoteCol(c)+" = ?")
			args = append(args, v)
		}
		if len(set) > 0 {
			if _, err := tx.Exec(`UPDATE npcs SET `+strings.Join(set, ", ")+` WHERE id = ?`, append(args, to)...); err != nil {
				return m, err
			}
			after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, to)
			if err != nil {
				return m, err
			}
			if _, err := auditRowChange(tx, who, "update", "npcs", map[string]any{"id": to}, before, after, detail); err != nil {
				return m, err
			}
			m.Tables = append(m.Tables, "npcs")
		}
	}

	if kind != MirrorDelete {
		for _, x := range npcMirrorCopied() {
			if _, here := extras[x.Table]; !here {
				continue // not a table here
			}
			changed, err := syncExtraRow(tx, who, x.Table, x.Key, to, extras[x.Table], detail)
			if err != nil {
				return m, fmt.Errorf("%s: %w", x.Table, err)
			}
			if changed {
				m.Tables = append(m.Tables, x.Table)
			}
		}
	}
	if len(m.Tables) > 0 {
		if _, err := tx.Exec(`INSERT INTO npc_mirror_inbox (npc_row, gone, tables, at) VALUES (?, ?, ?, ?)`,
			to, gone, strings.Join(m.Tables, ","), time.Now().UTC().Format(time.RFC3339)); err != nil {
			return m, err
		}
	}
	return m, tx.Commit()
}

// insertRowValues inserts values into table, but the columns skip names and
// those the table does not have, and returns the new rowid.
func insertRowValues(tx *sql.Tx, table string, have []string, values map[string]any, skip ...string) (int64, error) {
	var cols, q []string
	var args []any
	for _, c := range have {
		v, ok := values[c]
		if !ok || hasString(skip, c) {
			continue
		}
		cols = append(cols, quoteCol(c))
		q = append(q, "?")
		args = append(args, v)
	}
	res, err := tx.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(q, ", ")+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// syncExtraRow makes table's row keyed to (the row in tx's database) as want
// is (a row of the other database, keyed by its own row; nil: none), audited.
// It reports whether anything changed.
func syncExtraRow(tx *sql.Tx, who GMEdit, table, key string, to int64, want map[string]any, detail string) (bool, error) {
	if !tableExists(tx, table) {
		if want == nil {
			return false, nil
		}
		return false, errors.New("no such table there")
	}
	have, err := fullRow(tx, `SELECT * FROM `+table+` WHERE `+quoteCol(key)+` = ?`, to)
	if err != nil {
		return false, err
	}
	switch {
	case want == nil && have == nil:
		return false, nil
	case want == nil:
		return auditedDelete(tx, who, table, key, to, detail)
	case have == nil:
		cols, err := tableColumns(tx, table)
		if err != nil {
			return false, err
		}
		row := make(map[string]any, len(want))
		for c, v := range want {
			row[c] = v
		}
		row[key] = to
		if _, err := insertRowValues(tx, table, cols, row); err != nil {
			return false, err
		}
		after, err := fullRow(tx, `SELECT * FROM `+table+` WHERE `+quoteCol(key)+` = ?`, to)
		if err != nil {
			return false, err
		}
		_, err = auditRowChange(tx, who, "insert", table, map[string]any{key: to}, nil, after, detail)
		return err == nil, err
	}
	var set []string
	var args []any
	cols := make([]string, 0, len(want))
	for c := range want {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	for _, c := range cols {
		if _, ok := have[c]; !ok || c == key || sameValue(want[c], have[c]) {
			continue
		}
		set = append(set, quoteCol(c)+" = ?")
		args = append(args, want[c])
	}
	if len(set) == 0 {
		return false, nil
	}
	if _, err := tx.Exec(`UPDATE `+table+` SET `+strings.Join(set, ", ")+` WHERE `+quoteCol(key)+` = ?`, append(args, to)...); err != nil {
		return false, err
	}
	after, err := fullRow(tx, `SELECT * FROM `+table+` WHERE `+quoteCol(key)+` = ?`, to)
	if err != nil {
		return false, err
	}
	_, err = auditRowChange(tx, who, "update", table, map[string]any{key: to}, have, after, detail)
	return err == nil, err
}

// NPCMirrorNote is a line of npc_mirror_inbox: a row another shard's mirror
// changed here, to be put into the running world.
type NPCMirrorNote struct {
	ID     int64
	Row    int64
	Gone   bool
	Tables []string
}

// TakeNPCMirrorInbox reads and deletes the inbox's lines, oldest first; none
// (and no error) when there is no inbox.
func (db *SQLiteDB) TakeNPCMirrorInbox() ([]NPCMirrorNote, error) {
	if !tableExists(db, "npc_mirror_inbox") {
		return nil, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, npc_row, gone, tables FROM npc_mirror_inbox ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []NPCMirrorNote
	for rows.Next() {
		var n NPCMirrorNote
		var gone int
		var tables string
		if err := rows.Scan(&n.ID, &n.Row, &gone, &tables); err != nil {
			rows.Close()
			return nil, err
		}
		n.Gone = gone != 0
		if tables != "" {
			n.Tables = strings.Split(tables, ",")
		}
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	if _, err := tx.Exec(`DELETE FROM npc_mirror_inbox WHERE id <= ?`, out[len(out)-1].ID); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

// NPCMirrorOf is the row in peer that row from of db is mirrored as, or 0.
func NPCMirrorOf(peer *SQLiteDB, from int64) (int64, error) {
	if !tableExists(peer, "npc_mirror") {
		return 0, nil
	}
	var to int64
	err := peer.QueryRow(`SELECT to_id FROM npc_mirror WHERE from_id = ?`, from).Scan(&to)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return to, err
}
