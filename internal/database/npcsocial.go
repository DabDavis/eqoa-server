package database

import "fmt"

// npcsocial: which NPCs come to the help of their own when one is attacked
// (entity/socialaggro.go), by NAME, as npcaggro and factionhits are (a GM's
// #spawn copy shares it). social 1: an NPC of the name helps; 0: never.
// faction: the faction, or any group name, it belongs to for this: it helps
// an NPC of the same, and an NPC attacked near it is of a faction by its own
// row, else by the faction its kill lowers most (factionhits); empty, its own
// is that too. radius: how near the attacked NPC it must stand, 0 the
// default (30, a spell's usual range). source: gm, or seed for
// tools/seed_npc_social.py's rows (its --undo deletes those).
const npcSocialSchema = `
	CREATE TABLE IF NOT EXISTS npcsocial (
		npc_name TEXT PRIMARY KEY,
		social   INTEGER NOT NULL DEFAULT 1,
		faction  TEXT NOT NULL DEFAULT '',
		radius   REAL NOT NULL DEFAULT 0,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCSocial makes table npcsocial (initSchema).
func (db *SQLiteDB) ensureNPCSocial() error {
	if _, err := db.Exec(npcSocialSchema); err != nil {
		return fmt.Errorf("npcsocial: %w", err)
	}
	return nil
}

// NPCSocialRule is one row of npcsocial.
type NPCSocialRule struct {
	Name    string
	Social  bool
	Faction string
	Radius  float64
}

// NPCSocialRules is every row of npcsocial.
func (db *SQLiteDB) NPCSocialRules() ([]NPCSocialRule, error) {
	rows, err := db.Query(`SELECT npc_name, social, faction, radius FROM npcsocial`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCSocialRule
	for rows.Next() {
		var r NPCSocialRule
		var social int
		if err := rows.Scan(&r.Name, &social, &r.Faction, &r.Radius); err != nil {
			return nil, err
		}
		r.Social = social != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
