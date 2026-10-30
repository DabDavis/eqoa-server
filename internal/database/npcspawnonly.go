package database

import "fmt"

// npcspawnonly: the NPCs that stand in no one's world, there only as the
// original a quest's spawn event, a GM trigger's or a GM's #spawn copies
// (entity/spawnonly.go; the user, 2026-10-30, of a quest's creature standing
// beside its giver before the quest made it: "should spawn after I complete the
// steps"). A row is the whole rule: every NPC of the name, loaded as any other,
// is in no view, attacks and helps no one and never wanders; its copies are in
// the world as before. The key is the name as npcclass keys it (NPCClassKey:
// any case, trimmed, no trailing '$'). source: gm, or questspawn for
// tools/fix_quest_spawns.py's rows (its --undo deletes those).
const npcSpawnOnlySchema = `
	CREATE TABLE IF NOT EXISTS npcspawnonly (
		name   TEXT PRIMARY KEY,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCSpawnOnly makes table npcspawnonly (initSchema).
func (db *SQLiteDB) ensureNPCSpawnOnly() error {
	if _, err := db.Exec(npcSpawnOnlySchema); err != nil {
		return fmt.Errorf("npcspawnonly: %w", err)
	}
	return nil
}

// NPCSpawnOnly is every name of npcspawnonly, by NPCClassKey of the name (a
// blank one left out).
func (db *SQLiteDB) NPCSpawnOnly() (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM npcspawnonly`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if k := NPCClassKey(name); k != "" {
			out[k] = true
		}
	}
	return out, rows.Err()
}
