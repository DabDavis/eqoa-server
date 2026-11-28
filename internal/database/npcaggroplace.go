package database

import "fmt"

// npcaggroplace: a GM's word on the creatures of one name that spawned in one
// place, over npcaggro's word on the name (entity/aggro.go's aggroRuleLocked).
// aggressive 1: those that spawned within radius of (x, z) in world attack a
// player on sight; 0: never. The rest of the name's row (its radius, leash,
// see_invis, holds, greens) still holds for them. The smallest place holding a
// creature decides. By NAME as npcclass keys it (any case, trimmed, no
// trailing $).
//
// Heirloom petition 3508 (Sivi, 2026-11-28; the owner approved it as asked):
// "werewolfs in unkempt sw are not hostile". jib's video audit made the name
// werewolf peaceful everywhere (npcaggro, source video-audit: a white face on
// film); the Unkempt Woods' werewolves attack on sight, and his rule stays
// everywhere else.
const npcAggroPlaceSchema = `
	CREATE TABLE IF NOT EXISTS npcaggroplace (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		npc_name   TEXT NOT NULL,
		world      INTEGER NOT NULL DEFAULT 0,
		x          REAL NOT NULL DEFAULT 0,
		z          REAL NOT NULL DEFAULT 0,
		radius     REAL NOT NULL DEFAULT 0,
		aggressive INTEGER NOT NULL DEFAULT 1,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm',
		UNIQUE (npc_name, world, x, z, radius)
	);`

// ensureNPCAggroPlace makes table npcaggroplace (initSchema).
func (db *SQLiteDB) ensureNPCAggroPlace() error {
	if _, err := db.Exec(npcAggroPlaceSchema); err != nil {
		return fmt.Errorf("npcaggroplace: %w", err)
	}
	return nil
}

// NPCAggroPlace is one row of npcaggroplace.
type NPCAggroPlace struct {
	Name         string
	World        int
	X, Z, Radius float64
	Aggressive   bool
}

// NPCAggroPlaces is every row of npcaggroplace with a radius above 0 (a row of
// none holds nothing).
func (db *SQLiteDB) NPCAggroPlaces() ([]NPCAggroPlace, error) {
	rows, err := db.Query(`SELECT npc_name, world, x, z, radius, aggressive FROM npcaggroplace WHERE radius > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCAggroPlace
	for rows.Next() {
		var r NPCAggroPlace
		var aggressive int
		if err := rows.Scan(&r.Name, &r.World, &r.X, &r.Z, &r.Radius, &aggressive); err != nil {
			return nil, err
		}
		r.Aggressive = aggressive > 0
		out = append(out, r)
	}
	return out, rows.Err()
}
