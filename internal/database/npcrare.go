package database

import "fmt"

// npcrare: a rare spawn on a placeholder (the server owner, 2026-11-13: "wehali
// myrmidon in kerran isle is a placeholder" for Lord Koreshaa). By the
// placeholder's npcs row: each time that row's creature comes back from a death
// it comes back as the rare (an NPC name: the row of that name, the lowest id,
// is what it is made from, on the placeholder's spot) chance percent of the
// time, while no other creature of the rare's name stands alive in the world;
// otherwise as its own row. The rare is never on a clock of its own: its
// original row stands out of the world (npcspawnonly by its row, "#id"), and
// the placeholder's spot comes back after it as the placeholder's respawn time
// says (entity/npcrare.go). chance above 0 and at most 100; a row outside that
// counts as none.
const npcRareSchema = `
	CREATE TABLE IF NOT EXISTS npcrare (
		npc_row INTEGER PRIMARY KEY,
		rare    TEXT NOT NULL,
		chance  REAL NOT NULL DEFAULT 10,
		note    TEXT NOT NULL DEFAULT '',
		source  TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCRare makes table npcrare (initSchema).
func (db *SQLiteDB) ensureNPCRare() error {
	if _, err := db.Exec(npcRareSchema); err != nil {
		return fmt.Errorf("npcrare: %w", err)
	}
	return nil
}

// NPCRare is one row of npcrare that counts.
type NPCRare struct {
	NPCRow int
	Rare   string
	Chance float64 // percent, (0, 100]
}

// NPCRares is every row of npcrare that counts (a rare named, its chance above 0
// and at most 100), by placeholder row.
func (db *SQLiteDB) NPCRares() ([]NPCRare, error) {
	rows, err := db.Query(`SELECT npc_row, rare, chance FROM npcrare
		WHERE trim(rare) <> '' AND chance > 0 AND chance <= 100 ORDER BY npc_row`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCRare
	for rows.Next() {
		var r NPCRare
		if err := rows.Scan(&r.NPCRow, &r.Rare, &r.Chance); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// NPCRowNamed is the lowest npcs row whose name is name (NPCClassKey: any case,
// trimmed, no trailing '$'); 0 for none.
func (db *SQLiteDB) NPCRowNamed(name string) (int, error) {
	var id int
	err := db.QueryRow(`SELECT coalesce(min(id), 0) FROM npcs
		WHERE lower(trim(rtrim(trim(npc_name), '$'))) = ?`, NPCClassKey(name)).Scan(&id)
	return id, err
}
