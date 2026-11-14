package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
)

// gmnpcmove.go — a GM's /move in the game (handlers/gmnpcrow.go): an npcs
// row's spawn point and facing set to the GM's, with a wander of its own if
// asked (/move roamer, wander <radius>, stay), each row's change with its
// gm_audit line in the move's own transaction (revertible in the panel), and
// /move undo putting the GM's last move back from those lines. The wander
// alone (SetNPCWanderAudited) is what /clone roamer and /place ... roamer add
// to their copy.

// npcMovePrefix begins the audit detail of a /move's lines; /move undo finds
// them by it.
const npcMovePrefix = "/move in the game: "

// RoamerRadius is a roamer's wander radius when the GM gives none (the
// owner, 2026-11-12: "/clone roamer to make a roamer mob"): a far roamer's,
// past the server's 70, so it walks the longer legs (entity/roam.go).
const RoamerRadius = 300

// MaxWanderRadius is the widest wander a GM's command sets.
const MaxWanderRadius = 2000

// NPCWanderSet is a wander a GM's command gives a row: Clear takes its
// npcwander row away (/... stay: it moves as its name's row or its kind
// says); otherwise its npcwander row gets Radius.
type NPCWanderSet struct {
	Clear bool
	// Stand is a row of its own that stands its creature on its spawn whatever its
	// name's or its kind's rule (radius -1: NPCWanderRows), /roamer off
	Stand  bool
	Radius float64
	Note   string
}

// npcPlace are the columns /move sets.
var npcPlace = []string{"x_coord", "y_coord", "z_coord", "facing", "world"}

// setNPCWanderTx gives row id the wander w (audited with detail) and reports
// whether anything changed.
func setNPCWanderTx(tx *sql.Tx, who GMEdit, id int64, w NPCWanderSet, detail string) (bool, error) {
	if _, err := tx.Exec(npcWanderSchema); err != nil {
		return false, err
	}
	before, err := fullRow(tx, `SELECT * FROM npcwander WHERE npcid = ?`, id)
	if err != nil {
		return false, err
	}
	key := map[string]any{"npcid": id}
	if w.Clear {
		if before == nil {
			return false, nil
		}
		if _, err := tx.Exec(`DELETE FROM npcwander WHERE npcid = ?`, id); err != nil {
			return false, err
		}
		_, err := auditRowChange(tx, who, "delete", "npcwander", key, before, nil, detail)
		return err == nil, err
	}
	if w.Stand {
		w.Radius = -1
	} else if w.Radius <= 0 || w.Radius > MaxWanderRadius || math.IsNaN(w.Radius) {
		return false, fmt.Errorf("a wander radius is 1 to %d", MaxWanderRadius)
	}
	action := "insert"
	if before != nil {
		action = "update"
		if r, _ := before["radius"].(float64); r == w.Radius {
			return false, nil
		}
		if _, err := tx.Exec(`UPDATE npcwander SET radius = ?, note = ?, source = 'gm' WHERE npcid = ?`, w.Radius, w.Note, id); err != nil {
			return false, err
		}
	} else if _, err := tx.Exec(`INSERT INTO npcwander (npcid, radius, note, source) VALUES (?, ?, ?, 'gm')`, id, w.Radius, w.Note); err != nil {
		return false, err
	}
	after, err := fullRow(tx, `SELECT * FROM npcwander WHERE npcid = ?`, id)
	if err != nil {
		return false, err
	}
	_, err = auditRowChange(tx, who, action, "npcwander", key, before, after, detail)
	return err == nil, err
}

// SetNPCWanderAudited gives npcs row id the wander w, its audit line's
// detail detail; it reports whether npcwander changed.
func (db *SQLiteDB) SetNPCWanderAudited(id int64, w NPCWanderSet, who GMEdit, detail string) (bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	changed, err := setNPCWanderTx(tx, who, id, w, detail)
	if err != nil {
		return false, err
	}
	return changed, tx.Commit()
}

// NPCMoved is what MoveNPCRowAudited did: where the row stood, and whether
// its npcwander row changed.
type NPCMoved struct {
	From   NPCClone
	Wander bool
}

// MoveNPCRowAudited sets npcs row id's spawn point, facing and world to at
// (no kill time: it stands there now) and, when w is not nil, its wander, in
// one transaction with an audit line a row (detail "/move in the game: " +
// what and the spots). sql.ErrNoRows when there is no such row.
func (db *SQLiteDB) MoveNPCRowAudited(id int64, at NPCClone, w *NPCWanderSet, who GMEdit, what string) (NPCMoved, error) {
	var m NPCMoved
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return m, err
	}
	tx, err := db.Begin()
	if err != nil {
		return m, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	before, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return m, err
	}
	if before == nil {
		return m, sql.ErrNoRows
	}
	m.From = NPCClone{World: int(num(before["world"])), X: num(before["x_coord"]), Y: num(before["y_coord"]),
		Z: num(before["z_coord"]), Facing: int(num(before["facing"]))}
	set := `x_coord = ?, y_coord = ?, z_coord = ?, facing = ?, world = ?`
	if _, ok := before["dt_kill"]; ok {
		set += `, dt_kill = NULL`
	}
	if _, err := tx.Exec(`UPDATE npcs SET `+set+` WHERE id = ?`, at.X, at.Y, at.Z, at.Facing, at.World, id); err != nil {
		return m, err
	}
	after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return m, err
	}
	detail := fmt.Sprintf("%s%s from (%.1f, %.1f, %.1f) world %d facing %d to (%.1f, %.1f, %.1f) world %d facing %d",
		npcMovePrefix, what, m.From.X, m.From.Y, m.From.Z, m.From.World, m.From.Facing, at.X, at.Y, at.Z, at.World, at.Facing)
	if w != nil {
		if w.Clear {
			detail += ", its own wander taken away"
		} else {
			detail += fmt.Sprintf(", wandering %g", w.Radius)
		}
	}
	if _, err := auditRowChange(tx, who, "update", "npcs", map[string]any{"id": id}, before, after, detail); err != nil {
		return m, err
	}
	if w != nil {
		if m.Wander, err = setNPCWanderTx(tx, who, id, *w, detail); err != nil {
			return m, err
		}
	}
	return m, tx.Commit()
}

// num is an audit or fullRow value as a float64 (0 for none).
func num(v any) float64 {
	switch x := v.(type) {
	case int64:
		return float64(x)
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

// NPCMoveUndone is what UndoLastNPCMove put back.
type NPCMoveUndone struct {
	ID     int64
	Name   string
	Wander bool // its npcwander row was put back too
}

// UndoLastNPCMove puts back the account's last /move not undone yet: the npcs
// row's spot, facing and world, and its npcwander row as it was, each with
// its own audit line, the move's lines marked reverted by them. False when
// there is none. A row moved or changed since (its spot or its wander not as
// the move left them) is refused.
func (db *SQLiteDB) UndoLastNPCMove(accountID int, who GMEdit) (NPCMoveUndone, bool, error) {
	var r NPCMoveUndone
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return r, false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return r, false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	var lineID int64
	var detail, beforeS, afterS string
	err = tx.QueryRow(`SELECT id, detail, before, after FROM gm_audit WHERE accountid = ? AND action = 'update' AND tbl = 'npcs'
		AND detail LIKE ? AND reverted_by IS NULL ORDER BY id DESC LIMIT 1`, accountID, npcMovePrefix+"%").
		Scan(&lineID, &detail, &beforeS, &afterS)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	before, err := auditRowOf(beforeS)
	if err != nil {
		return r, false, fmt.Errorf("audit line %d: %w", lineID, err)
	}
	after, err := auditRowOf(afterS)
	if err != nil {
		return r, false, fmt.Errorf("audit line %d: %w", lineID, err)
	}
	r.ID, _ = before["id"].(int64)
	r.Name = fmt.Sprint(before["npc_name"])
	now, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, r.ID)
	if err != nil {
		return r, false, err
	}
	if now == nil {
		return r, false, fmt.Errorf("%s (row %d) is deleted since: nothing put back", r.Name, r.ID)
	}
	for _, c := range npcPlace {
		if num(now[c]) != num(after[c]) {
			return r, false, fmt.Errorf("%s (row %d) is moved or changed since (its %s): put it back in the GM panel", r.Name, r.ID, c)
		}
	}
	undo := "/move undo in the game: " + strings.TrimPrefix(detail, npcMovePrefix)
	set := make([]string, len(npcPlace))
	args := make([]any, 0, len(npcPlace)+1)
	for i, c := range npcPlace {
		set[i] = c + " = ?"
		args = append(args, before[c])
	}
	if _, err := tx.Exec(`UPDATE npcs SET `+strings.Join(set, ", ")+` WHERE id = ?`, append(args, r.ID)...); err != nil {
		return r, false, err
	}
	back, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, r.ID)
	if err != nil {
		return r, false, err
	}
	nid, err := auditRowChange(tx, who, "update", "npcs", map[string]any{"id": r.ID}, now, back, undo)
	if err != nil {
		return r, false, err
	}
	if _, err := tx.Exec(`UPDATE gm_audit SET reverted_by = ? WHERE id = ?`, nid, lineID); err != nil {
		return r, false, err
	}
	// its wander's line, if the move changed it: the next line, of the same detail
	var wID int64
	var wAction string
	var wBefore, wAfter sql.NullString
	err = tx.QueryRow(`SELECT id, action, before, after FROM gm_audit WHERE id = ? AND accountid = ? AND tbl = 'npcwander'
		AND detail = ? AND reverted_by IS NULL`, lineID+1, accountID, detail).Scan(&wID, &wAction, &wBefore, &wAfter)
	if errors.Is(err, sql.ErrNoRows) {
		return r, true, tx.Commit()
	}
	if err != nil {
		return r, false, err
	}
	if _, err := tx.Exec(npcWanderSchema); err != nil {
		return r, false, err
	}
	wNow, err := fullRow(tx, `SELECT * FROM npcwander WHERE npcid = ?`, r.ID)
	if err != nil {
		return r, false, err
	}
	var wWas map[string]any
	if wAfter.Valid {
		if wWas, err = auditRowOf(wAfter.String); err != nil {
			return r, false, err
		}
	}
	if (wNow == nil) != (wWas == nil) || wNow != nil && num(wNow["radius"]) != num(wWas["radius"]) {
		return r, false, fmt.Errorf("%s (row %d)'s wander is changed since: put it back in the GM panel", r.Name, r.ID)
	}
	if _, err := tx.Exec(`DELETE FROM npcwander WHERE npcid = ?`, r.ID); err != nil {
		return r, false, err
	}
	var wBack map[string]any
	if wBefore.Valid {
		row, err := auditRowOf(wBefore.String)
		if err != nil {
			return r, false, err
		}
		if err := insertRowMap(tx, "npcwander", row); err != nil {
			return r, false, err
		}
		if wBack, err = fullRow(tx, `SELECT * FROM npcwander WHERE npcid = ?`, r.ID); err != nil {
			return r, false, err
		}
	}
	action := "update"
	switch {
	case wNow == nil:
		action = "insert"
	case wBack == nil:
		action = "delete"
	}
	nid, err = auditRowChange(tx, who, action, "npcwander", map[string]any{"npcid": r.ID}, wNow, wBack, undo)
	if err != nil {
		return r, false, err
	}
	if _, err := tx.Exec(`UPDATE gm_audit SET reverted_by = ? WHERE id = ?`, nid, wID); err != nil {
		return r, false, err
	}
	r.Wander = true
	return r, true, tx.Commit()
}

// NPCRowName is the name of an npcs row called name (any case), as the row
// spells it, and whether there is one: /place's name not standing in the
// world now.
func (db *SQLiteDB) NPCRowName(name string) (string, bool) {
	var n string
	err := db.QueryRow(`SELECT npc_name FROM npcs WHERE npc_name = ? COLLATE NOCASE ORDER BY id LIMIT 1`, strings.TrimSpace(name)).Scan(&n)
	return n, err == nil
}

// SetNPCRowSizeAudited sets npcs row id's size, with an audit line (detail
// "/size in the game: " + what): a GM's /size on a creature kept over its
// respawns and restarts (the owner, 2026-11-13: "size may not being
// preserved across restarts"). sql.ErrNoRows when there is no such row.
func (db *SQLiteDB) SetNPCRowSizeAudited(id int64, size float64, who GMEdit, what string) error {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	before, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if before == nil {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`UPDATE npcs SET size = ? WHERE id = ?`, size, id); err != nil {
		return err
	}
	after, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	detail := fmt.Sprintf("/size in the game: %s from %.2f to %.2f", what, num(before["size"]), size)
	if _, err := auditRowChange(tx, who, "update", "npcs", map[string]any{"id": id}, before, after, detail); err != nil {
		return err
	}
	return tx.Commit()
}
