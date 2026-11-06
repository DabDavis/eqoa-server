package database

import "fmt"

// npcrespawn: a GM's word on how long one NPC takes to come back after its corpse goes,
// where the world's one figure (world.respawn_after, 60 s) is wrong for it (petition 1133,
// Jibmage: "Crocs on crocodile island are repopping way too fast"). By the npcs row, not its
// name, since a name may stand in many places. seconds above 0; a row whose seconds are 0
// or less counts as none.
const npcRespawnSchema = `
	CREATE TABLE IF NOT EXISTS npcrespawn (
		npc_row INTEGER PRIMARY KEY,
		seconds REAL NOT NULL,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCRespawn makes table npcrespawn (initSchema).
func (db *SQLiteDB) ensureNPCRespawn() error {
	if _, err := db.Exec(npcRespawnSchema); err != nil {
		return fmt.Errorf("npcrespawn: %w", err)
	}
	return nil
}

// NPCRespawns is every row of npcrespawn that counts: npcs row to seconds.
func (db *SQLiteDB) NPCRespawns() (map[uint32]float64, error) {
	rows, err := db.Query(`SELECT npc_row, seconds FROM npcrespawn WHERE seconds > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]float64{}
	for rows.Next() {
		var id uint32
		var s float64
		if err := rows.Scan(&id, &s); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}
