package database

import (
	"database/sql"
	"fmt"
)

// spellarea: a GM's word on a spell's area (entity/area.go; jib's review, F3
// pass 19), by spellid, over what the spell's line reads. centre: 0 as its
// line reads, 1 round its target, 2 round its caster, -1 not an area spell;
// radius: how far round it (0: its line's, else its kind's: 15 round a
// target, 10 round a harmful spell's caster); maxtargets: the most it lands
// on, its own target among them (0: the server's 8). A spell with no row is
// as its line reads. Read at every cast (handlers/cast.go).
const spellAreaSchema = `
	CREATE TABLE IF NOT EXISTS spellarea (
		spellid    INTEGER PRIMARY KEY,
		centre     INTEGER NOT NULL DEFAULT 0,
		radius     REAL NOT NULL DEFAULT 0,
		maxtargets INTEGER NOT NULL DEFAULT 0,
		note       TEXT NOT NULL DEFAULT ''
	);`

// ensureSpellArea makes table spellarea (initSchema).
func (db *SQLiteDB) ensureSpellArea() error {
	if _, err := db.Exec(spellAreaSchema); err != nil {
		return fmt.Errorf("spellarea: %w", err)
	}
	return nil
}

// SpellAreaRow is a spell's row of spellarea.
type SpellAreaRow struct {
	Centre     int
	Radius     float64
	MaxTargets int
}

// SpellArea is a spell's spellarea row; ok is false for a spell with none.
func (s *SpellbookStore) SpellArea(spellID int) (row SpellAreaRow, ok bool, err error) {
	err = s.db.QueryRow(`SELECT centre, radius, maxtargets FROM spellarea WHERE spellid = ?`, spellID).
		Scan(&row.Centre, &row.Radius, &row.MaxTargets)
	if err == sql.ErrNoRows {
		return SpellAreaRow{}, false, nil
	}
	if err != nil {
		return SpellAreaRow{}, false, err
	}
	return row, true, nil
}
