package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A GM's marks, camps and paths from the game (handlers/gmcamps.go; the GM
// Jibrish, 2026-11-08: "maybe a command to capture multiple points? so i can
// go HERE, HERE and HERE", "or at least static spawns", and petition 1293's
// rock, whose four static spawns popped a child or a pariah of Xrag): a GM
// stands at a spot and marks it (/mark), and the marks become a camp's static
// spawns (/camp), a creature's path (/path), or the points a GM's petition
// carries to whoever reads it (its server view).
//
//   - gmmarks: one row a mark, by account, in the order made (its id): where
//     the GM's character stood (world, x, y, z, and facing in radians as the
//     server holds it), a note, the character and the time. Marks outlive a
//     logout and a restart; /camp and /path take them all and clear them.
//   - gmcamps: one row a /camp, by account: its world, the kinds given (NPC
//     names, ", " between), the marks it took (JSON, which /camp undo gives
//     back), when and by which character.
//   - campspawns: one row a camp's spawn, by its npcs row: its camp and the
//     kinds it pops (NPC names, ", " between), one at random each time it comes
//     back (entity/campspawn.go). Its npcs row is its spot (place, facing,
//     world) and the kind it was made as.
//
// Every write here is a row change of the GM panel's kind: its gm_audit line
// (the row before and after, keyed as the panel keys the table) in the
// change's own transaction, so the panel lists it and can revert it.

// MaxGMMarks is the most marks an account keeps: a path's most points.
const MaxGMMarks = MaxPathPoints

// MaxCampKinds is the most kinds one camp's spawns pop.
const MaxCampKinds = 8

// ensureCamps makes the marks' and the camps' tables (initSchema).
func (db *SQLiteDB) ensureCamps() error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS gmmarks (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid INTEGER NOT NULL,
		world     INTEGER NOT NULL DEFAULT 0,
		x         REAL NOT NULL DEFAULT 0,
		y         REAL NOT NULL DEFAULT 0,
		z         REAL NOT NULL DEFAULT 0,
		facing    REAL NOT NULL DEFAULT 0,
		note      TEXT NOT NULL DEFAULT '',
		charname  TEXT NOT NULL DEFAULT '',
		at        TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_gmmarks_account ON gmmarks(accountid, id);
	CREATE TABLE IF NOT EXISTS gmcamps (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		accountid INTEGER NOT NULL,
		charname  TEXT NOT NULL DEFAULT '',
		world     INTEGER NOT NULL DEFAULT 0,
		kinds     TEXT NOT NULL DEFAULT '',
		marks     TEXT NOT NULL DEFAULT '[]',
		at        TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS campspawns (
		npcid INTEGER PRIMARY KEY,
		camp  INTEGER NOT NULL DEFAULT 0,
		kinds TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_campspawns_camp ON campspawns(camp);`)
	return err
}

// GMMark is one of a GM's marks.
type GMMark struct {
	ID        int64   `json:"id"`
	AccountID int     `json:"-"`
	World     int     `json:"world"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	Facing    float64 `json:"facing"`
	Note      string  `json:"note,omitempty"`
	CharName  string  `json:"char,omitempty"`
	At        string  `json:"at,omitempty"`
}

// CampKindNames is a kinds column's names: split at the commas, each trimmed,
// none empty.
func CampKindNames(s string) []string {
	var out []string
	for _, n := range strings.Split(s, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// CampKindsText is names as a kinds column keeps them.
func CampKindsText(names []string) string {
	return strings.Join(names, ", ")
}

// fillWho names who's account as the audit log names a GM, when the caller
// did not.
func fillWho(tx *sql.Tx, who *GMEdit) {
	if who.Username != "" {
		return
	}
	if err := tx.QueryRow(`SELECT username FROM accountinfo WHERE accountid = ?`, who.AccountID).Scan(&who.Username); err != nil {
		who.Username = fmt.Sprintf("account %d", who.AccountID)
	}
}

// auditedInsert runs an insert of one row into table, whose key column is
// keyCol, and writes its audit line; the row's key is the insert's rowid. It
// returns the key.
func auditedInsert(tx *sql.Tx, who GMEdit, table, keyCol, detail, query string, args ...any) (int64, error) {
	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	after, err := fullRow(tx, `SELECT * FROM `+table+` WHERE `+keyCol+` = ?`, id)
	if err != nil {
		return 0, err
	}
	if _, err := auditRowChange(tx, who, "insert", table, map[string]any{keyCol: id}, nil, after, detail); err != nil {
		return 0, err
	}
	return id, nil
}

// auditedDelete deletes the row of table whose keyCol is key, with its audit
// line; false when there was none.
func auditedDelete(tx *sql.Tx, who GMEdit, table, keyCol string, key int64, detail string) (bool, error) {
	before, err := fullRow(tx, `SELECT * FROM `+table+` WHERE `+keyCol+` = ?`, key)
	if err != nil || before == nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE `+keyCol+` = ?`, key); err != nil {
		return false, err
	}
	if _, err := auditRowChange(tx, who, "delete", table, map[string]any{keyCol: key}, before, nil, detail); err != nil {
		return false, err
	}
	return true, nil
}

// marksTx is an account's marks inside tx, in the order made.
func marksTx(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, accountID int) ([]GMMark, error) {
	rows, err := q.Query(`SELECT id, accountid, world, x, y, z, facing, note, charname, at FROM gmmarks
		WHERE accountid = ? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GMMark
	for rows.Next() {
		var m GMMark
		if err := rows.Scan(&m.ID, &m.AccountID, &m.World, &m.X, &m.Y, &m.Z, &m.Facing, &m.Note, &m.CharName, &m.At); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GMMarks is an account's marks, in the order made (mark 1 first).
func (db *SQLiteDB) GMMarks(accountID int) ([]GMMark, error) {
	return marksTx(db, accountID)
}

// errTooManyMarks is AddGMMark's refusal past MaxGMMarks.
var errTooManyMarks = fmt.Errorf("%d marks are the most kept: /camp or /path takes them, /marks clear drops them", MaxGMMarks)

// ErrTooManyMarks reports AddGMMark's refusal of a mark past MaxGMMarks.
func ErrTooManyMarks(err error) bool { return errors.Is(err, errTooManyMarks) }

// insertMarkTx keeps m as its account's next mark, audited.
func insertMarkTx(tx *sql.Tx, who GMEdit, m GMMark, detail string) (int64, error) {
	if m.At == "" {
		m.At = time.Now().UTC().Format(time.RFC3339)
	}
	return auditedInsert(tx, who, "gmmarks", "id", detail,
		`INSERT INTO gmmarks (accountid, world, x, y, z, facing, note, charname, at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.AccountID, m.World, m.X, m.Y, m.Z, m.Facing, m.Note, m.CharName, m.At)
}

// AddGMMark keeps m as its account's next mark (its id and time set here),
// audited, and returns it with its number (1 for the first an account holds).
// Past MaxGMMarks it is refused (ErrTooManyMarks).
func (db *SQLiteDB) AddGMMark(m GMMark, who GMEdit) (GMMark, int, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return m, 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return m, 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM gmmarks WHERE accountid = ?`, m.AccountID).Scan(&n); err != nil {
		return m, 0, err
	}
	if n >= MaxGMMarks {
		return m, 0, errTooManyMarks
	}
	m.At = time.Now().UTC().Format(time.RFC3339)
	if m.ID, err = insertMarkTx(tx, who, m, fmt.Sprintf("/mark in the game: mark %d", n+1)); err != nil {
		return m, 0, err
	}
	return m, n + 1, tx.Commit()
}

// DropLastGMMark drops an account's last mark, audited: the mark, its number,
// and false when it had none.
func (db *SQLiteDB) DropLastGMMark(accountID int, who GMEdit) (GMMark, int, bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return GMMark{}, 0, false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return GMMark{}, 0, false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	marks, err := marksTx(tx, accountID)
	if err != nil || len(marks) == 0 {
		return GMMark{}, 0, false, err
	}
	last := marks[len(marks)-1]
	if _, err := auditedDelete(tx, who, "gmmarks", "id", last.ID, fmt.Sprintf("/marks undo in the game: mark %d", len(marks))); err != nil {
		return GMMark{}, 0, false, err
	}
	return last, len(marks), true, tx.Commit()
}

// clearMarksTx drops every mark of an account inside tx, audited, and says
// how many.
func clearMarksTx(tx *sql.Tx, who GMEdit, accountID int, detail string) (int, error) {
	marks, err := marksTx(tx, accountID)
	if err != nil {
		return 0, err
	}
	for _, m := range marks {
		if _, err := auditedDelete(tx, who, "gmmarks", "id", m.ID, detail); err != nil {
			return 0, err
		}
	}
	return len(marks), nil
}

// ClearGMMarks drops every mark of an account, audited, and says how many.
func (db *SQLiteDB) ClearGMMarks(accountID int, who GMEdit) (int, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, err
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	n, err := clearMarksTx(tx, who, accountID, "/marks clear in the game")
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// sameMarks is whether the account's marks in tx are exactly want, by id:
// what a command planned from is what it takes.
func sameMarks(tx *sql.Tx, accountID int, want []GMMark) error {
	now, err := marksTx(tx, accountID)
	if err != nil {
		return err
	}
	if len(now) != len(want) {
		return errors.New("your marks changed meanwhile: try again")
	}
	for i := range now {
		if now[i].ID != want[i].ID {
			return errors.New("your marks changed meanwhile: try again")
		}
	}
	return nil
}

// CampSpawnRow is a row of campspawns.
type CampSpawnRow struct {
	NPCID int
	Camp  int64
	Kinds []string
}

// CampSpawns is every row of campspawns, by npcs row.
func (db *SQLiteDB) CampSpawns() ([]CampSpawnRow, error) {
	rows, err := db.Query(`SELECT npcid, camp, kinds FROM campspawns ORDER BY npcid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampSpawnRow
	for rows.Next() {
		var r CampSpawnRow
		var kinds string
		if err := rows.Scan(&r.NPCID, &r.Camp, &kinds); err != nil {
			return nil, err
		}
		r.Kinds = CampKindNames(kinds)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CampSpawnPlan is one spawn a camp makes: the npcs row it copies (the kind it
// is made as) and its spot, its facing as the row keeps it (0-255).
type CampSpawnPlan struct {
	From    int
	World   int
	X, Y, Z float64
	Facing  int
}

// CampPlan is a /camp: whose, by which character, its world, the kinds its
// spawns pop (NPC names as the rows spell them), its spawns, and the marks
// they stand at (all the account's, cleared; kept on the camp's row).
type CampPlan struct {
	AccountID int
	CharName  string
	World     int
	Kinds     []string
	Spawns    []CampSpawnPlan
	Marks     []GMMark
}

// CampMade is what MakeCamp made: the camp's id and each spawn's npcs row, in
// the plan's order.
type CampMade struct {
	Camp   int64
	NPCIDs []int
}

// npcCopyColumns is every column of npcs but its id, as a copy takes them.
func npcCopyColumns(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`PRAGMA table_info(npcs)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		if name != "id" {
			cols = append(cols, `"`+strings.ReplaceAll(name, `"`, `""`)+`"`)
		}
	}
	return cols, rows.Err()
}

// MakeCamp makes a camp (plan): its gmcamps row, each spawn's npcs row (a copy
// of its kind's whole row, as the content tools copy one, on its spot with its
// facing, no kill time) and its campspawns row, and the account's marks
// cleared, all in one transaction with an audit line a row. The marks must be
// the account's still, as planned.
func (db *SQLiteDB) MakeCamp(plan CampPlan, who GMEdit) (CampMade, error) {
	var made CampMade
	if len(plan.Spawns) == 0 || len(plan.Kinds) == 0 {
		return made, errors.New("a camp needs a spawn and a kind")
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return made, err
	}
	tx, err := db.Begin()
	if err != nil {
		return made, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	if err := sameMarks(tx, plan.AccountID, plan.Marks); err != nil {
		return made, err
	}
	marks, err := json.Marshal(plan.Marks)
	if err != nil {
		return made, err
	}
	kinds := CampKindsText(plan.Kinds)
	made.Camp, err = auditedInsert(tx, who, "gmcamps", "id", fmt.Sprintf("/camp in the game: %d spawns of %s", len(plan.Spawns), kinds),
		`INSERT INTO gmcamps (accountid, charname, world, kinds, marks, at) VALUES (?, ?, ?, ?, ?, ?)`,
		plan.AccountID, plan.CharName, plan.World, kinds, string(marks), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return made, err
	}
	cols, err := npcCopyColumns(tx)
	if err != nil {
		return made, err
	}
	list := strings.Join(cols, ", ")
	hasKill := strings.Contains(list, `"dt_kill"`)
	for i, sp := range plan.Spawns {
		res, err := tx.Exec(`INSERT INTO npcs (`+list+`) SELECT `+list+` FROM npcs WHERE id = ?`, sp.From)
		if err != nil {
			return made, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return made, fmt.Errorf("NPC row %d is gone: nothing to copy", sp.From)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return made, err
		}
		set := `x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, world = ?`
		if hasKill {
			set += `, dt_kill = NULL`
		}
		if _, err := tx.Exec(`UPDATE npcs SET `+set+` WHERE id = ?`, sp.X, sp.Y, sp.Z, sp.Facing, sp.World, id); err != nil {
			return made, err
		}
		after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
		if err != nil {
			return made, err
		}
		detail := fmt.Sprintf("/camp in the game: camp %d's spawn %d, a copy of NPC %d", made.Camp, i+1, sp.From)
		if _, err := auditRowChange(tx, who, "insert", "npcs", map[string]any{"id": id}, nil, after, detail); err != nil {
			return made, err
		}
		if _, err := auditedInsert(tx, who, "campspawns", "npcid", detail,
			`INSERT INTO campspawns (npcid, camp, kinds) VALUES (?, ?, ?)`, id, made.Camp, kinds); err != nil {
			return made, err
		}
		made.NPCIDs = append(made.NPCIDs, int(id))
	}
	if _, err := clearMarksTx(tx, who, plan.AccountID, fmt.Sprintf("/camp in the game: camp %d took them", made.Camp)); err != nil {
		return made, err
	}
	return made, tx.Commit()
}

// CampUndone is what UndoLastCamp took away: the camp, its kinds, its spawns'
// npcs rows deleted and those that had a path, and how many of its marks went
// back to the account (none when it held marks of its own).
type CampUndone struct {
	Camp      int64
	Kinds     string
	NPCIDs    []int
	Paths     []int
	MarksBack int
	MarksHeld int // the account's own marks, which kept the camp's from coming back
}

// UndoLastCamp takes away the account's last camp: each spawn's npcs row, its
// path and its campspawns row, then the camp's row, in one transaction with an
// audit line a row; the camp's marks are the account's again when it holds
// none. False when the account has made no camp still standing.
func (db *SQLiteDB) UndoLastCamp(accountID int, who GMEdit) (CampUndone, bool, error) {
	var u CampUndone
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return u, false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return u, false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	var marks string
	err = tx.QueryRow(`SELECT id, kinds, marks FROM gmcamps WHERE accountid = ? ORDER BY id DESC LIMIT 1`, accountID).
		Scan(&u.Camp, &u.Kinds, &marks)
	if errors.Is(err, sql.ErrNoRows) {
		return u, false, nil
	}
	if err != nil {
		return u, false, err
	}
	rows, err := tx.Query(`SELECT npcid FROM campspawns WHERE camp = ? ORDER BY npcid`, u.Camp)
	if err != nil {
		return u, false, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return u, false, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, false, err
	}
	detail := fmt.Sprintf("/camp undo in the game: camp %d", u.Camp)
	for _, id := range ids {
		if gone, err := auditedDelete(tx, who, "npcpaths", "npcid", id, detail); err != nil {
			return u, false, err
		} else if gone {
			u.Paths = append(u.Paths, int(id))
		}
		if gone, err := auditedDelete(tx, who, "npcs", "id", id, detail); err != nil {
			return u, false, err
		} else if gone {
			u.NPCIDs = append(u.NPCIDs, int(id))
		}
		if _, err := auditedDelete(tx, who, "campspawns", "npcid", id, detail); err != nil {
			return u, false, err
		}
	}
	if _, err := auditedDelete(tx, who, "gmcamps", "id", u.Camp, detail); err != nil {
		return u, false, err
	}
	own, err := marksTx(tx, accountID)
	if err != nil {
		return u, false, err
	}
	u.MarksHeld = len(own)
	var back []GMMark
	if marks != "" {
		if err := json.Unmarshal([]byte(marks), &back); err != nil {
			back = nil // a row edited by hand: the camp goes, its marks do not come back
		}
	}
	if len(own) == 0 {
		for i, m := range back {
			if i == MaxGMMarks {
				break
			}
			m.AccountID = accountID
			if _, err := insertMarkTx(tx, who, m, detail+": its marks given back"); err != nil {
				return u, false, err
			}
			u.MarksBack++
		}
	}
	return u, true, tx.Commit()
}

// SetNPCPathAudited writes an NPC's path (its npcpaths row, held to Check) as
// the GM panel's set_npc_path writes one, with its audit line in the same
// transaction, and then clears the marks of the account marksOf (0: none). It
// reports what the row's change was: insert, update or unchanged.
func (db *SQLiteDB) SetNPCPathAudited(rec NPCPathRecord, who GMEdit, detail string, marksOf int, marks []GMMark) (string, error) {
	if err := rec.Check(); err != nil {
		return "", err
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	if marksOf != 0 {
		if err := sameMarks(tx, marksOf, marks); err != nil {
			return "", err
		}
	}
	const sel = `SELECT * FROM npcpaths WHERE npcid = ?`
	before, err := fullRow(tx, sel, rec.NPCID)
	if err != nil {
		return "", err
	}
	points := PathPointsJSON(rec.Points)
	action := "insert"
	if before == nil {
		_, err = tx.Exec(`INSERT INTO npcpaths (npcid, world, mode, pause, speed, points) VALUES (?, ?, ?, ?, ?, ?)`,
			rec.NPCID, rec.World, rec.Mode, rec.Pause, rec.Speed, points)
	} else {
		action = "update"
		_, err = tx.Exec(`UPDATE npcpaths SET world = ?, mode = ?, pause = ?, speed = ?, points = ? WHERE npcid = ?`,
			rec.World, rec.Mode, rec.Pause, rec.Speed, points, rec.NPCID)
	}
	if err != nil {
		return "", err
	}
	after, err := fullRow(tx, sel, rec.NPCID)
	if err != nil {
		return "", err
	}
	if before != nil && fmt.Sprint(before) == fmt.Sprint(after) {
		action = "unchanged"
	} else if _, err := auditRowChange(tx, who, action, "npcpaths", map[string]any{"npcid": int64(rec.NPCID)}, before, after, detail); err != nil {
		return "", err
	}
	if marksOf != 0 {
		if _, err := clearMarksTx(tx, who, marksOf, detail+": the marks it took"); err != nil {
			return "", err
		}
	}
	return action, tx.Commit()
}

// ClearNPCPathAudited takes an NPC's path away (its npcpaths row), audited;
// false when it had none.
func (db *SQLiteDB) ClearNPCPathAudited(npcID int, who GMEdit, detail string) (bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	gone, err := auditedDelete(tx, who, "npcpaths", "npcid", int64(npcID), detail)
	if err != nil || !gone {
		return false, err
	}
	return true, tx.Commit()
}
