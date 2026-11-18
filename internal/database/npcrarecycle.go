package database

import "fmt"

// npcrarecycle: an NPC that is only sometimes there, by its npcs row (the owner,
// 2026-11-18, the forum's Siliskor quest: "Zaryn Boome, he spawns rarely", in
// Blackwater by the first well). A placeholder's rare (npcrare.go) comes back
// from a death; a townsman who gives a quest never dies, so this one keeps a
// clock of its own instead (entity/npcrarecycle.go): it stands for up_minutes,
// then is gone (out of every view) for a time drawn between gone_min and
// gone_max minutes, then stands again, and so on. At a start it is gone for a
// time drawn between 0 and gone_max. up_minutes above 0, gone_min at least 0,
// gone_max at least gone_min and above 0; a row outside that counts as none.
const npcRareCycleSchema = `
	CREATE TABLE IF NOT EXISTS npcrarecycle (
		npc_row    INTEGER PRIMARY KEY,
		up_minutes REAL NOT NULL DEFAULT 30,
		gone_min   REAL NOT NULL DEFAULT 60,
		gone_max   REAL NOT NULL DEFAULT 180,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCRareCycle makes table npcrarecycle (initSchema).
func (db *SQLiteDB) ensureNPCRareCycle() error {
	if _, err := db.Exec(npcRareCycleSchema); err != nil {
		return fmt.Errorf("npcrarecycle: %w", err)
	}
	return nil
}

// NPCRareCycle is one row of npcrarecycle that counts.
type NPCRareCycle struct {
	NPCRow                      int
	UpMinutes, GoneMin, GoneMax float64
}

// NPCRareCycles is every row of npcrarecycle that counts, by row.
func (db *SQLiteDB) NPCRareCycles() ([]NPCRareCycle, error) {
	rows, err := db.Query(`SELECT npc_row, up_minutes, gone_min, gone_max FROM npcrarecycle
		WHERE up_minutes > 0 AND gone_min >= 0 AND gone_max >= gone_min AND gone_max > 0 ORDER BY npc_row`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCRareCycle
	for rows.Next() {
		var r NPCRareCycle
		if err := rows.Scan(&r.NPCRow, &r.UpMinutes, &r.GoneMin, &r.GoneMax); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
