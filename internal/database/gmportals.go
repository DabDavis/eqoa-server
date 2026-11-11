package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// A GM's portals (handlers/gmportal.go, petition 1624: the GM Jibrish, "a GM
// command that allows me to mark the entry and exit points of teleporters"):
// /markportal names an NPC of a row as a portal's entry, /markportal exit its
// landing, and the pair is kept here, off, with the ticket that asks for it to
// be built. A row switched on (/markportal on, or enabled = 1 here) makes its
// NPC ask "Do you wish to travel to <place>?" Yes or No, a waysender's trip
// (handlers/waysender.go) to the landing. Every change has its gm_audit line.

const gmPortalsSchema = `
	CREATE TABLE IF NOT EXISTS gmportals (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		npc_id      INTEGER NOT NULL,
		npc_name    TEXT NOT NULL DEFAULT '',
		entry_world INTEGER NOT NULL DEFAULT 0,
		entry_x     REAL NOT NULL DEFAULT 0,
		entry_y     REAL NOT NULL DEFAULT 0,
		entry_z     REAL NOT NULL DEFAULT 0,
		place       TEXT NOT NULL DEFAULT '',
		world       INTEGER NOT NULL DEFAULT 0,
		x           REAL NOT NULL DEFAULT 0,
		y           REAL NOT NULL DEFAULT 0,
		z           REAL NOT NULL DEFAULT 0,
		facing      REAL NOT NULL DEFAULT 0,
		exit_by     TEXT NOT NULL DEFAULT '',
		exit_name   TEXT NOT NULL DEFAULT '',
		exit_npc_id INTEGER NOT NULL DEFAULT 0,
		fee         INTEGER NOT NULL DEFAULT 0,
		min_level   INTEGER NOT NULL DEFAULT 0,
		enabled     INTEGER NOT NULL DEFAULT 0,
		petition    INTEGER NOT NULL DEFAULT 0,
		accountid   INTEGER NOT NULL DEFAULT 0,
		charname    TEXT NOT NULL DEFAULT '',
		created     TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_gmportals_npc ON gmportals(npc_id);`

// ensureGMPortals makes table gmportals (initSchema).
func (db *SQLiteDB) ensureGMPortals() error {
	if _, err := db.Exec(gmPortalsSchema); err != nil {
		return fmt.Errorf("gmportals: %w", err)
	}
	return nil
}

// GMPortal is a row of gmportals: the entry NPC (its npcs row, name and spot
// when marked), the landing (a place's name as the question says it, the
// world, the point and the heading in radians), how the landing was taken
// ("target": the GM's target's spot; "stand": where the GM stood) and the
// target's name and npcs row (0: a player, or none), a fee and a level (0:
// none), whether it is on, its ticket, and who marked it when.
type GMPortal struct {
	ID                     int64
	NPCID                  int
	NPCName                string
	EntryWorld             int
	EntryX, EntryY, EntryZ float64
	Place                  string
	World                  int
	X, Y, Z, Facing        float64
	ExitBy                 string
	ExitName               string
	ExitNPCID              int
	Fee, MinLevel          int
	Enabled                bool
	Petition               int64
	AccountID              int
	CharName               string
	Created                string
}

const gmPortalCols = `id, npc_id, npc_name, entry_world, entry_x, entry_y, entry_z, place, world, x, y, z, facing,
	exit_by, exit_name, exit_npc_id, fee, min_level, enabled, petition, accountid, charname, created`

func scanGMPortal(r interface{ Scan(...any) error }) (GMPortal, error) {
	var p GMPortal
	err := r.Scan(&p.ID, &p.NPCID, &p.NPCName, &p.EntryWorld, &p.EntryX, &p.EntryY, &p.EntryZ, &p.Place, &p.World,
		&p.X, &p.Y, &p.Z, &p.Facing, &p.ExitBy, &p.ExitName, &p.ExitNPCID, &p.Fee, &p.MinLevel, &p.Enabled,
		&p.Petition, &p.AccountID, &p.CharName, &p.Created)
	return p, err
}

func (db *SQLiteDB) gmPortals(where string, args ...any) ([]GMPortal, error) {
	rows, err := db.Query(`SELECT `+gmPortalCols+` FROM gmportals `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GMPortal
	for rows.Next() {
		p, err := scanGMPortal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EnabledGMPortals is every portal switched on, in id order.
func (db *SQLiteDB) EnabledGMPortals() ([]GMPortal, error) {
	return db.gmPortals(`WHERE enabled <> 0 ORDER BY id`)
}

// RecentGMPortals is the last n portals marked, the newest first.
func (db *SQLiteDB) RecentGMPortals(n int) ([]GMPortal, error) {
	return db.gmPortals(`ORDER BY id DESC LIMIT ?`, n)
}

// ErrNoGMPortal is a portal id with no row.
var ErrNoGMPortal = errors.New("no such portal")

// GMPortalByID is the portal of that id (ErrNoGMPortal for none).
func (db *SQLiteDB) GMPortalByID(id int64) (GMPortal, error) {
	p, err := scanGMPortal(db.QueryRow(`SELECT `+gmPortalCols+` FROM gmportals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNoGMPortal
	}
	return p, err
}

// AddGMPortal keeps p as a new row (its id, created and the rest as given),
// audited; it returns the row as kept.
func (db *SQLiteDB) AddGMPortal(p GMPortal, who GMEdit, detail string) (GMPortal, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return p, err
	}
	tx, err := db.Begin()
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	if p.Created == "" {
		p.Created = time.Now().UTC().Format(time.RFC3339)
	}
	id, err := auditedInsert(tx, who, "gmportals", "id", detail,
		`INSERT INTO gmportals (npc_id, npc_name, entry_world, entry_x, entry_y, entry_z, place, world, x, y, z, facing,
			exit_by, exit_name, exit_npc_id, fee, min_level, enabled, petition, accountid, charname, created)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.NPCID, p.NPCName, p.EntryWorld, p.EntryX, p.EntryY, p.EntryZ, p.Place, p.World, p.X, p.Y, p.Z, p.Facing,
		p.ExitBy, p.ExitName, p.ExitNPCID, p.Fee, p.MinLevel, p.Enabled, p.Petition, p.AccountID, p.CharName, p.Created)
	if err != nil {
		return p, err
	}
	p.ID = id
	return p, tx.Commit()
}

// updateGMPortal sets one column of portal id to v, audited, and returns the
// row after; changed is false (and nothing written) when it already held v.
func (db *SQLiteDB) updateGMPortal(id int64, col string, v any, who GMEdit, detail string) (GMPortal, bool, error) {
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return GMPortal{}, false, err
	}
	tx, err := db.Begin()
	if err != nil {
		return GMPortal{}, false, err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	before, err := fullRow(tx, `SELECT * FROM gmportals WHERE id = ?`, id)
	if err != nil {
		return GMPortal{}, false, err
	}
	if before == nil {
		return GMPortal{}, false, ErrNoGMPortal
	}
	res, err := tx.Exec(`UPDATE gmportals SET `+col+` = ? WHERE id = ? AND `+col+` IS NOT ?`, v, id, v)
	if err != nil {
		return GMPortal{}, false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return GMPortal{}, false, err
	}
	if n > 0 {
		after, err := fullRow(tx, `SELECT * FROM gmportals WHERE id = ?`, id)
		if err != nil {
			return GMPortal{}, false, err
		}
		if _, err := auditRowChange(tx, who, "update", "gmportals", map[string]any{"id": id}, before, after, detail); err != nil {
			return GMPortal{}, false, err
		}
	}
	p, err := scanGMPortal(tx.QueryRow(`SELECT `+gmPortalCols+` FROM gmportals WHERE id = ?`, id))
	if err != nil {
		return p, false, err
	}
	return p, n > 0, tx.Commit()
}

// SetGMPortalEnabled switches portal id on or off, audited: the row after, and
// whether it changed.
func (db *SQLiteDB) SetGMPortalEnabled(id int64, on bool, who GMEdit, detail string) (GMPortal, bool, error) {
	v := 0
	if on {
		v = 1
	}
	return db.updateGMPortal(id, "enabled", v, who, detail)
}

// SetGMPortalPetition keeps portal id's ticket, audited.
func (db *SQLiteDB) SetGMPortalPetition(id, petition int64, who GMEdit, detail string) (GMPortal, error) {
	p, _, err := db.updateGMPortal(id, "petition", petition, who, detail)
	return p, err
}
