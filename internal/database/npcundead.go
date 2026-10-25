package database

import "fmt"

// npcundead: which creatures are undead, by NPC name (F3 20261025-215937,
// jib: "should only work on monsters tagged Undead"), for the spells that
// work on nothing else and the charms that never take one (entity/undead.go).
// undead 1: every NPC of the name is undead; 0: none is, whatever its name
// says. The key is the name as npcclass keys it (NPCClassKey: lower case,
// trimmed, no trailing '$'), so a GM's #spawn copy shares it. source: gm, or
// seed for tools/seed_npc_undead.py's rows (its --undo deletes those).
const npcUndeadSchema = `
	CREATE TABLE IF NOT EXISTS npcundead (
		name   TEXT PRIMARY KEY,
		undead INTEGER NOT NULL DEFAULT 1,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCUndead makes table npcundead (initSchema).
func (db *SQLiteDB) ensureNPCUndead() error {
	if _, err := db.Exec(npcUndeadSchema); err != nil {
		return fmt.Errorf("npcundead: %w", err)
	}
	return nil
}

// NPCUndeadTags is every row of npcundead: whether the NPCs of a name are
// undead, by NPCClassKey of the name.
func (db *SQLiteDB) NPCUndeadTags() (map[string]bool, error) {
	rows, err := db.Query(`SELECT name, undead FROM npcundead`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		var undead int
		if err := rows.Scan(&name, &undead); err != nil {
			return nil, err
		}
		out[NPCClassKey(name)] = undead != 0
	}
	return out, rows.Err()
}
