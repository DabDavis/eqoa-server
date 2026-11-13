package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// gmnpcrow.go — a GM's /delete and /clone in the game (handlers/gmnpcrow.go):
// an npcs row deleted with the rows keyed by it, or copied with them onto the
// GM's spot, each row's change with its gm_audit line in the change's own
// transaction (as the GM panel writes one: revertible there), and /delete undo
// putting the GM's last delete back from those lines.

// npcRowExtras are the tables keyed by an npcs row's id that are the row's
// own: deleted with it, and (but npcpaths, whose points are where the row
// stood, and campspawns, which would make a copy one of a camp's) copied
// with it. Name-keyed tables (npcaggro, npcclass, npcloot, ...) are the
// name's, every row's of that name: untouched. npcitem is keyed by npc_id, a
// catalog the rows of a merchant share (a copy keeps the same npc_id).
var npcRowExtras = []struct {
	Table, Key string
	Copy       bool
}{
	{"npcrespawn", "npc_row", true},
	{"npcwander", "npcid", true},
	{"npctitle", "npcid", true},
	{"signtext", "npcid", true},
	{"npcpaths", "npcid", false},
	{"campspawns", "npcid", false},
	{"npcstands", "npc_row", true}, // a spawn-only name's row that stands (npcstands.go)
}

// npcDeletePrefix begins the audit detail of a /delete's lines; /delete undo
// finds them by it.
const npcDeletePrefix = "/delete in the game: "

// ErrNPCRowInUse is a row /delete refuses: something else names it by its id.
type ErrNPCRowInUse struct{ Uses []string }

func (e *ErrNPCRowInUse) Error() string {
	return "it is named by " + strings.Join(e.Uses, "; ") + " (change those in the GM panel first)"
}

// tableExists reports whether table is in the database.
func tableExists(q interface {
	QueryRow(string, ...any) *sql.Row
}, table string) bool {
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
	return err == nil && n > 0
}

// npcRowUses is what else names an npcs row by its id, which a delete would
// leave pointing at nothing: a quest step's NPC, a quest's wall, a GM
// portal's entry or exit.
func npcRowUses(tx *sql.Tx, id int64) ([]string, error) {
	checks := []struct{ table, query, what string }{
		{"queststeps", `SELECT questid || '/' || step FROM queststeps WHERE npcid = ?`, "quest step"},
		{"questwalls", `SELECT questid || '/' || step FROM questwalls WHERE npcid = ?`, "quest wall (quest/step)"},
		{"gmportals", `SELECT id FROM gmportals WHERE npc_id = ?1 OR exit_npc_id = ?1`, "GM portal"},
	}
	var uses []string
	for _, c := range checks {
		if !tableExists(tx, c.table) {
			continue
		}
		rows, err := tx.Query(c.query, id)
		if err != nil {
			return nil, err
		}
		var keys []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return nil, err
			}
			keys = append(keys, k)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			uses = append(uses, fmt.Sprintf("%s %s", c.what, strings.Join(keys, ", ")))
		}
	}
	return uses, nil
}

// NPCRowDeleted is what DeleteNPCRowAudited took away: the row and the
// tables of its extras that had one.
type NPCRowDeleted struct {
	Row    map[string]any
	Extras []string
}

// DeleteNPCRowAudited deletes npcs row id and its extras (npcRowExtras), in
// one transaction with an audit line a row (detail: npcDeletePrefix + what).
// A row another table names (npcRowUses) is refused (*ErrNPCRowInUse);
// sql.ErrNoRows when there is no such row.
func (db *SQLiteDB) DeleteNPCRowAudited(id int64, who GMEdit, what string) (NPCRowDeleted, error) {
	var d NPCRowDeleted
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return d, err
	}
	tx, err := db.Begin()
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	row, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return d, err
	}
	if row == nil {
		return d, sql.ErrNoRows
	}
	uses, err := npcRowUses(tx, id)
	if err != nil {
		return d, err
	}
	if len(uses) > 0 {
		return d, &ErrNPCRowInUse{Uses: uses}
	}
	detail := npcDeletePrefix + what
	for _, x := range npcRowExtras {
		if !tableExists(tx, x.Table) {
			continue
		}
		gone, err := auditedDelete(tx, who, x.Table, x.Key, id, detail)
		if err != nil {
			return d, fmt.Errorf("%s: %w", x.Table, err)
		}
		if gone {
			d.Extras = append(d.Extras, x.Table)
		}
	}
	if _, err := auditedDelete(tx, who, "npcs", "id", id, detail); err != nil {
		return d, err
	}
	d.Row = row
	return d, tx.Commit()
}

// NPCRowRestored is what UndoLastNPCDelete put back.
type NPCRowRestored struct {
	ID     int64
	Name   string
	Tables []string // npcs first, then its extras
	By     string   // who deleted it (gm_audit's username)
}

func quoteCol(c string) string { return `"` + strings.ReplaceAll(c, `"`, `""`) + `"` }

// insertRowMap inserts row (column -> value, as an audit line keeps it) into
// table; a column the table no longer has is refused.
func insertRowMap(tx *sql.Tx, table string, row map[string]any) error {
	all, err := tableColumns(tx, table)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range all {
		have[c] = true
	}
	cols := make([]string, 0, len(row))
	for c := range row {
		if !have[c] {
			return fmt.Errorf("%s has no column %q now", table, c)
		}
		cols = append(cols, c)
	}
	sort.Strings(cols)
	q := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		q[i] = "?"
		args[i] = row[c]
		cols[i] = quoteCol(c)
	}
	_, err = tx.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(q, ", ")+`)`, args...)
	return err
}

// auditRowOf is a row as an audit line keeps it (JSON), each whole number an
// int64, any other number a float64.
func auditRowOf(s string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var row map[string]any
	if err := dec.Decode(&row); err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errors.New("no row")
	}
	for k, v := range row {
		if n, ok := v.(json.Number); ok {
			if i, err := n.Int64(); err == nil {
				row[k] = i
			} else if f, err := n.Float64(); err == nil {
				row[k] = f
			}
		}
	}
	return row, nil
}

// NPCDeletesToUndo is how many /deletes, any GM's, are not undone yet: how many more /delete undo puts back, each the
// one before the last (the owner, 2026-11-12: "/delete undo it will undo last delete and keep going if press again";
// "let any gm undo anyones delete"). accountID is unused: every GM's count.
func (db *SQLiteDB) NPCDeletesToUndo(accountID int) (int, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, err
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE action = 'delete' AND tbl = 'npcs'
		AND detail LIKE ? AND reverted_by IS NULL`, npcDeletePrefix+"%").Scan(&n)
	return n, err
}

// UndoLastNPCDelete puts back the last /delete not undone yet, any GM's (the owner, 2026-11-12: "let any gm undo
// anyones delete"; accountID is unused): the
// npcs row and its extras, from the delete's audit lines, each with its own
// insert line, the delete's lines marked reverted by them. False when there
// is none. A row whose id is taken again (a later insert) is refused.
func (db *SQLiteDB) UndoLastNPCDelete(accountID int, who GMEdit) (NPCRowRestored, bool, error) {
	var r NPCRowRestored
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return r, false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return r, false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	var detail string
	var delAccount int
	var by sql.NullString
	err = tx.QueryRow(`SELECT detail, accountid, username FROM gm_audit WHERE action = 'delete' AND tbl = 'npcs'
		AND detail LIKE ? AND reverted_by IS NULL ORDER BY id DESC LIMIT 1`, npcDeletePrefix+"%").Scan(&detail, &delAccount, &by)
	r.By = by.String
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	type line struct {
		id     int64
		table  string
		before string
	}
	rows, err := tx.Query(`SELECT id, tbl, before FROM gm_audit WHERE accountid = ? AND action = 'delete' AND detail = ?
		AND reverted_by IS NULL ORDER BY id DESC`, delAccount, detail)
	if err != nil {
		return r, false, err
	}
	var lines []line
	for rows.Next() {
		var l line
		var b sql.NullString
		if err := rows.Scan(&l.id, &l.table, &b); err != nil {
			rows.Close()
			return r, false, err
		}
		l.before = b.String
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return r, false, err
	}
	// the npcs row first (its lines were written last), then its extras
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].table == "npcs" && lines[j].table != "npcs" })
	undo := "/delete undo in the game: " + strings.TrimPrefix(detail, npcDeletePrefix)
	for _, l := range lines {
		row, err := auditRowOf(l.before)
		if err != nil {
			return r, false, fmt.Errorf("audit line %d: its row unreadable: %w", l.id, err)
		}
		key := "id"
		for _, x := range npcRowExtras {
			if x.Table == l.table {
				key = x.Key
			}
		}
		if l.table == "npcs" {
			id, _ := row["id"].(int64)
			r.ID = id
			r.Name = fmt.Sprint(row["npc_name"])
			var n int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM npcs WHERE id = ?`, r.ID).Scan(&n); err != nil {
				return r, false, err
			}
			if n > 0 {
				return r, false, fmt.Errorf("NPC row %d is taken again (a later insert): put it back in the GM panel", r.ID)
			}
		}
		if err := insertRowMap(tx, l.table, row); err != nil {
			return r, false, fmt.Errorf("%s: %w", l.table, err)
		}
		after, err := fullRow(tx, `SELECT * FROM `+l.table+` WHERE `+key+` = ?`, row[key])
		if err != nil {
			return r, false, err
		}
		nid, err := auditRowChange(tx, who, "insert", l.table, map[string]any{key: row[key]}, nil, after, undo)
		if err != nil {
			return r, false, err
		}
		if _, err := tx.Exec(`UPDATE gm_audit SET reverted_by = ? WHERE id = ?`, nid, l.id); err != nil {
			return r, false, err
		}
		r.Tables = append(r.Tables, l.table)
	}
	return r, true, tx.Commit()
}

// NPCClone is where CloneNPCRowAudited puts its copy: the world, the spot and
// the facing (0-255, as npcs keeps it).
type NPCClone struct {
	World   int
	X, Y, Z float64
	Facing  int
}

// CloneNPCRowAudited copies npcs row from (every column but its id, as /camp
// copies one) onto at, no kill time, and its copied extras (npcRowExtras) to
// the new row, in one transaction with an audit line a row (detail
// "/clone in the game: " + what). It returns the new row's id and the extras'
// tables copied; sql.ErrNoRows when there is no such row.
func (db *SQLiteDB) CloneNPCRowAudited(from int64, at NPCClone, who GMEdit, what string) (int64, []string, error) {
	return db.copyNPCRowAudited(from, at, who, fmt.Sprintf("/clone in the game: %s, a copy of NPC %d", what, from), "")
}

// PlaceNPCRowAudited is /place's copy: as CloneNPCRowAudited, its audit
// detail "/place in the game: ". stands is a note for an npcstands row given
// the new row when its name stands out of the world (npcspawnonly): the copy
// stands in it, audited in the same transaction ("" for none; one copied from
// the source row is kept as it is).
func (db *SQLiteDB) PlaceNPCRowAudited(from int64, at NPCClone, who GMEdit, what, stands string) (int64, []string, error) {
	return db.copyNPCRowAudited(from, at, who, fmt.Sprintf("/place in the game: %s, a copy of NPC %d", what, from), stands)
}

// PlaceSourceRow is the npcs row /place copies for a name (any case): of
// those in world if any, else of all, the most common body and level (most
// rows; a tie the body and level of the lowest row), its lowest row. 0 and
// sql.ErrNoRows when no row has the name.
func (db *SQLiteDB) PlaceSourceRow(name string, world int) (int64, error) {
	rows, err := db.Query(`SELECT id, world, COALESCE(modelid, 0), COALESCE(npc_level, 0) FROM npcs
		WHERE npc_name = ? COLLATE NOCASE ORDER BY id`, name)
	if err != nil {
		return 0, err
	}
	type cand struct{ id, world, model, level int64 }
	var all, here []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.world, &c.model, &c.level); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, c)
		if c.world == int64(world) {
			here = append(here, c)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(here) > 0 {
		all = here
	}
	if len(all) == 0 {
		return 0, sql.ErrNoRows
	}
	type kind struct{ model, level int64 }
	n := map[kind]int{}
	first := map[kind]int64{}
	for _, c := range all {
		k := kind{c.model, c.level}
		if n[k] == 0 {
			first[k] = c.id
		}
		n[k]++
	}
	best := all[0]
	bk := kind{best.model, best.level}
	for k, c := range n {
		if c > n[bk] || c == n[bk] && first[k] < first[bk] {
			bk = k
		}
	}
	return first[bk], nil
}

func (db *SQLiteDB) copyNPCRowAudited(from int64, at NPCClone, who GMEdit, detail, stands string) (int64, []string, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return 0, nil, err
	}
	if stands != "" {
		if err := db.ensureNPCStands(); err != nil {
			return 0, nil, err
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	cols, err := npcCopyColumns(tx)
	if err != nil {
		return 0, nil, err
	}
	list := strings.Join(cols, ", ")
	res, err := tx.Exec(`INSERT INTO npcs (`+list+`) SELECT `+list+` FROM npcs WHERE id = ?`, from)
	if err != nil {
		return 0, nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, nil, sql.ErrNoRows
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, nil, err
	}
	set := `x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, world = ?`
	if strings.Contains(list, `"dt_kill"`) {
		set += `, dt_kill = NULL`
	}
	if _, err := tx.Exec(`UPDATE npcs SET `+set+` WHERE id = ?`, at.X, at.Y, at.Z, at.Facing, at.World, id); err != nil {
		return 0, nil, err
	}
	after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return 0, nil, err
	}
	if _, err := auditRowChange(tx, who, "insert", "npcs", map[string]any{"id": id}, nil, after, detail); err != nil {
		return 0, nil, err
	}
	var copied []string
	for _, x := range npcRowExtras {
		if !x.Copy || !tableExists(tx, x.Table) {
			continue
		}
		all, err := tableColumns(tx, x.Table)
		if err != nil {
			return 0, nil, err
		}
		var rest []string
		for _, c := range all {
			if c != x.Key {
				rest = append(rest, quoteCol(c))
			}
		}
		sel := "?"
		ins := quoteCol(x.Key)
		if len(rest) > 0 {
			sel += ", " + strings.Join(rest, ", ")
			ins += ", " + strings.Join(rest, ", ")
		}
		res, err := tx.Exec(`INSERT INTO `+x.Table+` (`+ins+`) SELECT `+sel+` FROM `+x.Table+` WHERE `+quoteCol(x.Key)+` = ?`, id, from)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", x.Table, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		row, err := fullRow(tx, `SELECT * FROM `+x.Table+` WHERE `+quoteCol(x.Key)+` = ?`, id)
		if err != nil {
			return 0, nil, err
		}
		if _, err := auditRowChange(tx, who, "insert", x.Table, map[string]any{x.Key: id}, nil, row, detail); err != nil {
			return 0, nil, err
		}
		copied = append(copied, x.Table)
	}
	if stands != "" && !hasString(copied, "npcstands") {
		if _, err := tx.Exec(`INSERT INTO npcstands (npc_row, note, source) VALUES (?, ?, 'gm')`, id, stands); err != nil {
			return 0, nil, fmt.Errorf("npcstands: %w", err)
		}
		row, err := fullRow(tx, `SELECT * FROM npcstands WHERE npc_row = ?`, id)
		if err != nil {
			return 0, nil, err
		}
		if _, err := auditRowChange(tx, who, "insert", "npcstands", map[string]any{"npc_row": id}, nil, row, detail); err != nil {
			return 0, nil, err
		}
		copied = append(copied, "npcstands")
	}
	return id, copied, tx.Commit()
}

func hasString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
