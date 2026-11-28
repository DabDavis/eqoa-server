package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// npchidden.go — a GM's /delete hides an NPC's row, never deletes it (the owner, 2026-11-28: "never allow ./delete to
// remove items/mobs/etc from database", then "./delete can remove from the shard like now, but we don't want the
// database to lose the data"). A hidden row (table npchidden, by its npcs id) is out of the shard as a deleted one was:
// the loaders skip it (GetAllNPCs, GetNPCsByZone, GetNPC: a hidden row reads as no row, so a camp's fill, a rare's
// placeholder, a GM portal, a mirror's update or a panel's lookup by row never brings it back), while the npcs row and
// every row keyed by it (npcrespawn, npcwander, npctitle, signtext, npcpaths, campspawns, npcstands) stay as they were.
// /delete undo takes the mark off (UndoLastNPCDelete) and the row stands again.

// NPCHiddenSchema is table npchidden: an npcs row a GM's /delete took out of the shard, by whom, when, and why.
const NPCHiddenSchema = `CREATE TABLE IF NOT EXISTS npchidden (
	npc_row INTEGER PRIMARY KEY,
	by      TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL DEFAULT '',
	note    TEXT NOT NULL DEFAULT ''
)`

// npcNotHidden is the condition, on table npcs, that keeps a hidden row out.
const npcNotHidden = `NOT EXISTS (SELECT 1 FROM npchidden h WHERE h.npc_row = npcs.id)`

func (db *SQLiteDB) ensureNPCHidden() error {
	_, err := db.Exec(NPCHiddenSchema)
	return err
}

// ErrNPCRowHidden is a /delete of a row hidden already.
var ErrNPCRowHidden = errors.New("it is hidden already")

// NPCRowHidden reports whether npcs row id is hidden (a GM's /delete).
func (db *SQLiteDB) NPCRowHidden(id int64) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM npchidden WHERE npc_row = ?`, id).Scan(&n)
	return n > 0, err
}

// hideNPCRowTx hides npcs row id in tx with its gm_audit insert line (detail), refusing a row another table names
// (npcRowUses) and one hidden already. The row itself is read back for the caller.
func hideNPCRowTx(tx *sql.Tx, who GMEdit, id int64, detail, note string) (map[string]any, error) {
	if _, err := tx.Exec(NPCHiddenSchema); err != nil {
		return nil, err
	}
	row, err := fullRow(tx, `SELECT * FROM npcs WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, sql.ErrNoRows
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM npchidden WHERE npc_row = ?`, id).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, ErrNPCRowHidden
	}
	uses, err := npcRowUses(tx, id)
	if err != nil {
		return nil, err
	}
	if len(uses) > 0 {
		return nil, &ErrNPCRowInUse{Uses: uses}
	}
	if _, err := tx.Exec(`INSERT INTO npchidden (npc_row, by, at, note) VALUES (?, ?, ?, ?)`,
		id, who.Username, time.Now().UTC().Format(time.RFC3339), note); err != nil {
		return nil, err
	}
	after, err := fullRow(tx, `SELECT * FROM npchidden WHERE npc_row = ?`, id)
	if err != nil {
		return nil, err
	}
	if _, err := auditRowChange(tx, who, "insert", "npchidden", map[string]any{"npc_row": id}, nil, after, detail); err != nil {
		return nil, err
	}
	return row, nil
}

// unhideNPCRowTx takes npcs row id's hidden mark off in tx with its gm_audit delete line (detail), the hide's line
// (hideLine, 0 none) marked reverted by it.
func unhideNPCRowTx(tx *sql.Tx, who GMEdit, id int64, detail string, hideLine int64) error {
	before, err := fullRow(tx, `SELECT * FROM npchidden WHERE npc_row = ?`, id)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("row %d is not hidden", id)
	}
	if _, err := tx.Exec(`DELETE FROM npchidden WHERE npc_row = ?`, id); err != nil {
		return err
	}
	nid, err := auditRowChange(tx, who, "delete", "npchidden", map[string]any{"npc_row": id}, before, nil, detail)
	if err != nil {
		return err
	}
	if hideLine > 0 {
		if _, err := tx.Exec(`UPDATE gm_audit SET reverted_by = ? WHERE id = ?`, nid, hideLine); err != nil {
			return err
		}
	}
	return nil
}
