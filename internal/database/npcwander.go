package database

import "fmt"

// npcwander: how one creature wanders, by its npcs row, ahead of its name's
// npcmove row and its kind (entity/movement.go). The owner, 2026-11-12: "there
// are many zones with mobs not dispersed or patroling, lets fix that disperse
// and patrol"; the GM Jibrish's petitions 1734-1736 ("Spread out the wolfwere's
// here, make them roam"): a camp's few wanderers among its standing kind, or
// one camp's creatures roaming where the rest of their name stands, need a rule
// by the row, as a path is (npcpaths). A row is the whole rule: the creature
// wanders within radius of its spawn (0: as its name's row or the server's
// radius; below 0: it does not wander at all), whatever its name's row or its
// kind says, unless it is one that never wanders (a pet, a training dummy, a path's, a camp's static spawn, a
// spawn-only original, one with no body or the NPC bit, one off the ground).
// source: gm, or the tool that wrote it (tools/disperse_patrol_20261112.py:
// disperse, its --undo deletes its own).
const npcWanderSchema = `
	CREATE TABLE IF NOT EXISTS npcwander (
		npcid  INTEGER PRIMARY KEY,
		radius REAL NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCWander makes table npcwander (initSchema).
func (db *SQLiteDB) ensureNPCWander() error {
	if _, err := db.Exec(npcWanderSchema); err != nil {
		return fmt.Errorf("npcwander: %w", err)
	}
	return nil
}

// NPCWanderRows is every row of npcwander: the radius by the npcs row's id (0
// for its name's or the server's; a negative one, read as -1, for none: the
// creature stands on its spawn until pulled, whatever its name's or its kind,
// the GM Jibcrotwo's petitions 2088-2089: "He does not roam").
func (db *SQLiteDB) NPCWanderRows() (map[int64]float32, error) {
	rows, err := db.Query(`SELECT npcid, radius FROM npcwander`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]float32)
	for rows.Next() {
		var id int64
		var r float64
		if err := rows.Scan(&id, &r); err != nil {
			return nil, err
		}
		out[id] = float32(max(r, -1))
	}
	return out, rows.Err()
}
