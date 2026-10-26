package database

import "fmt"

// npcmove: how the creatures of a name move (F3 20261026-021730, jib:
// "hexbones in zentaurs keep do NOT roam, they are static spawn until agrod";
// F3 20261026-020316: "Wildlife that is not aquatic should avoid naturally
// roaming into the water ... Caiman, crocodiles, crabs can roam both"),
// read by entity/movement.go. roam 0: it stands on its spawn until a fight
// takes it (and goes back there after); 1, the default, it wanders. radius:
// how far from its spawn it wanders, 0 the server's (world.roam_radius).
// water: 'land' it keeps out of the water as it wanders; 'both' it goes in
// and out of it (crocodiles, caiman, crabs); 'water' it keeps to the water,
// wandering and fighting; empty, by its spawn (water when it spawned in the
// water, else land). The key is the name as npcclass keys it
// (NPCClassKey). source: gm, or seed for tools/seed_npc_move.py's rows (its
// --undo deletes those).
const npcMoveSchema = `
	CREATE TABLE IF NOT EXISTS npcmove (
		name   TEXT PRIMARY KEY,
		roam   INTEGER NOT NULL DEFAULT 1,
		radius REAL NOT NULL DEFAULT 0,
		water  TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCMove makes table npcmove (initSchema).
func (db *SQLiteDB) ensureNPCMove() error {
	if _, err := db.Exec(npcMoveSchema); err != nil {
		return fmt.Errorf("npcmove: %w", err)
	}
	return nil
}

// NPCMoveRule is a row of npcmove.
type NPCMoveRule struct {
	Roam   bool
	Radius float32
	Water  string // "" (by its spawn), "land", "both" or "water"
}

// NPCMoveRules is every row of npcmove, by NPCClassKey of the name. A water
// word it does not know counts as empty (by its spawn).
func (db *SQLiteDB) NPCMoveRules() (map[string]NPCMoveRule, error) {
	rows, err := db.Query(`SELECT name, roam, radius, water FROM npcmove`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]NPCMoveRule)
	for rows.Next() {
		var name, water string
		var roam int
		var radius float64
		if err := rows.Scan(&name, &roam, &radius, &water); err != nil {
			return nil, err
		}
		switch water {
		case "land", "both", "water":
		default:
			water = ""
		}
		out[NPCClassKey(name)] = NPCMoveRule{Roam: roam != 0, Radius: float32(max(radius, 0)), Water: water}
	}
	return out, rows.Err()
}
