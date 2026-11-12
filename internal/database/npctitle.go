package database

import "fmt"

// npctitle: a line over an NPC's name, by its npcs row (entity/npctitle.go;
// the GM Jibrish's petitions 1765 and 1766, 2026-11-12: "Give the testing
// heroes vendor a title like mercenary's have. Make it say "XP Grinding
// Rewards"", "Give dabrish a title "XP/CM's""). title is the line (24
// letters at most, as a name: the rest is cut); "" none. source: gm, or the
// tool that wrote it (tools/misc1112_20261112.py: misc1112, its --undo
// deletes its own).
const npcTitleSchema = `
	CREATE TABLE IF NOT EXISTS npctitle (
		npcid  INTEGER PRIMARY KEY,
		title  TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCTitle makes table npctitle (initSchema).
func (db *SQLiteDB) ensureNPCTitle() error {
	if _, err := db.Exec(npcTitleSchema); err != nil {
		return fmt.Errorf("npctitle: %w", err)
	}
	return nil
}

// NPCTitleRows is every row of npctitle with a title: the line by the npcs
// row's id.
func (db *SQLiteDB) NPCTitleRows() (map[int64]string, error) {
	rows, err := db.Query(`SELECT npcid, title FROM npctitle WHERE title != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var t string
		if err := rows.Scan(&id, &t); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}
