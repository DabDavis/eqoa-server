package database

import "fmt"

// npcaggro: a GM's word on the creatures of one name where entity/aggro.go's
// rule is wrong for them. aggressive 1: they attack a player on sight (a camp
// of bandits with players' race bodies, say); 0: never (a creature to be left
// peaceful). radius: how near a player must come, 0 their own (30). By NAME,
// as factionhits is: a GM's #spawn copy shares it. source: gm, or seed for
// tools/seed_npc_aggro.py's rows (its --undo deletes those).
const npcAggroSchema = `
	CREATE TABLE IF NOT EXISTS npcaggro (
		npc_name   TEXT PRIMARY KEY,
		aggressive INTEGER NOT NULL DEFAULT 1,
		radius     REAL NOT NULL DEFAULT 0,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCAggro makes table npcaggro (initSchema).
func (db *SQLiteDB) ensureNPCAggro() error {
	if _, err := db.Exec(npcAggroSchema); err != nil {
		return fmt.Errorf("npcaggro: %w", err)
	}
	return nil
}

// NPCAggroRule is one row of npcaggro.
type NPCAggroRule struct {
	Name       string
	Aggressive bool
	Radius     float64
	// Holding (column holding, questwalls.go's ensure): when not 0 the rule
	// is for the players holding that item alone: they are attacked on sight
	// (or not, by aggressive), every other player is left alone (the Estate's
	// Guardian and Cazic's Scent: "will become KoS and attack you if you have
	// Cazic's Scent on you", t196)
	Holding int
}

// NPCAggroRules is every row of npcaggro.
func (db *SQLiteDB) NPCAggroRules() ([]NPCAggroRule, error) {
	rows, err := db.Query(`SELECT npc_name, aggressive, radius, holding FROM npcaggro`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCAggroRule
	for rows.Next() {
		var r NPCAggroRule
		var aggressive int
		if err := rows.Scan(&r.Name, &aggressive, &r.Radius, &r.Holding); err != nil {
			return nil, err
		}
		r.Aggressive = aggressive != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
