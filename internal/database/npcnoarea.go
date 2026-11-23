package database

import "fmt"

// npcnoarea: the creatures no area spell touches, by NPC name (Heirloom
// petition 2740, 2026-11-23: "Unmarked Headstone in ZP is affected by AoE";
// entity/noarea.go). A creature of a name here is hit by what is cast at it
// alone: a player's area spells, an area potion's or thrown potion's splash,
// a pet's or a mercenary's area cast pass it by. The key is the name as
// npcclass keys it (NPCClassKey: lower case, trimmed, no trailing '$'), so a
// GM's #spawn copy shares it.
const npcNoAreaSchema = `
	CREATE TABLE IF NOT EXISTS npcnoarea (
		name   TEXT PRIMARY KEY,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCNoArea makes table npcnoarea (initSchema).
func (db *SQLiteDB) ensureNPCNoArea() error {
	if _, err := db.Exec(npcNoAreaSchema); err != nil {
		return fmt.Errorf("npcnoarea: %w", err)
	}
	return nil
}

// NPCNoAreaNames is every name of npcnoarea, by NPCClassKey.
func (db *SQLiteDB) NPCNoAreaNames() (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM npcnoarea`)
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
		out[NPCClassKey(name)] = true
	}
	return out, rows.Err()
}
