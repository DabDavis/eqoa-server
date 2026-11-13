package database

import "fmt"

// npcstands: an npcs row that stands in the world though its name is in
// npcspawnonly (entity/spawnonly.go). The owner, 2026-11-13: the GMs' /place
// filth squid in the Plane of Disease was refused, the name standing out of
// the world as the original quest 101289's step 4 copies; GMs want to place
// such creatures as standing spawns. /place of a spawn-only name writes one
// for its new row (handlers/gmnpcrow.go); it is one of the row's own extras
// (npcRowExtras): /delete takes it with the row, /delete undo puts it back,
// /clone copies it. Every other row of the name, and every quest's, trigger's
// and GM's spawn of it, is as before: the spawn-only original is still what
// they copy. A row's own npcspawnonly ("#id") and a dungeon's template are
// held out of the world all the same.
const npcStandsSchema = `
	CREATE TABLE IF NOT EXISTS npcstands (
		npc_row INTEGER PRIMARY KEY,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCStands makes table npcstands (initSchema).
func (db *SQLiteDB) ensureNPCStands() error {
	if _, err := db.Exec(npcStandsSchema); err != nil {
		return fmt.Errorf("npcstands: %w", err)
	}
	return nil
}

// NPCStands is every row of npcstands (a row above 0).
func (db *SQLiteDB) NPCStands() (map[int64]bool, error) {
	out := make(map[int64]bool)
	if !tableExists(db, "npcstands") {
		return out, nil
	}
	rows, err := db.Query(`SELECT npc_row FROM npcstands WHERE npc_row > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
