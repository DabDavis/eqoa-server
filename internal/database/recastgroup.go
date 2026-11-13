package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// spellrecastgroup: spells that share one recast (ticket 2011, Brother, 2026-11-13:
// the ranger's Wolf's Instinct and Hunter's Instinct, its no-form upgrade, both kept
// in the book by ticket 1241, each with its own 30 minute recast, so both were up at
// once). A spell's group is grp; the spells of one group are alternatives of each
// other: casting one locks out every one of them for the cast spell's own lockout
// (entity/recastgroup.go), and their lasting effects are one buff line, the newer
// taking the older off. A spell in no group has no row. Written by
// tools/recast_group_20261113.py, or a GM in the panel; read at each cast, so a
// change takes effect at the next cast with no restart.
const spellRecastGroupSchema = `
	CREATE TABLE IF NOT EXISTS spellrecastgroup (
		spellid INTEGER PRIMARY KEY,
		grp     TEXT NOT NULL
	);`

// ensureSpellRecastGroup makes table spellrecastgroup (initSchema).
func (db *SQLiteDB) ensureSpellRecastGroup() error {
	if _, err := db.Exec(spellRecastGroupSchema); err != nil {
		return fmt.Errorf("spellrecastgroup: %w", err)
	}
	return nil
}

// RecastGroup is a spell's recast group and every spell of it (the spell
// included, by id), "" and none for a spell in no group.
func (s *SpellbookStore) RecastGroup(spellID int) (string, []int, error) {
	var grp string
	err := s.db.QueryRow(`SELECT grp FROM spellrecastgroup WHERE spellid = ?`, spellID).Scan(&grp)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, err
	}
	grp = strings.TrimSpace(grp)
	if grp == "" {
		return "", nil, nil
	}
	rows, err := s.db.Query(`SELECT spellid FROM spellrecastgroup WHERE TRIM(grp) = ? ORDER BY spellid`, grp)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return "", nil, err
		}
		ids = append(ids, id)
	}
	return grp, ids, rows.Err()
}

// SetRecastGroup puts a spell in a recast group, "" taking it out of its own (tests and tools).
func (s *SpellbookStore) SetRecastGroup(spellID int, grp string) error {
	grp = strings.TrimSpace(grp)
	if grp == "" {
		_, err := s.db.Exec(`DELETE FROM spellrecastgroup WHERE spellid = ?`, spellID)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO spellrecastgroup (spellid, grp) VALUES (?, ?)
		ON CONFLICT (spellid) DO UPDATE SET grp = excluded.grp`, spellID, grp)
	return err
}
