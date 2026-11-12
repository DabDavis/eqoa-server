package database

import "fmt"

// npcaggro: a GM's word on the creatures of one name where entity/aggro.go's
// rule is wrong for them. aggressive 1: they attack a player on sight (a camp
// of bandits with players' race bodies, say); 0: never (a creature to be left
// peaceful); -1: no word on that (the world's rules decide, as with no row: a
// row for its leash alone). radius: how near a player must come, 0 their own
// (30). leash: how far from where a fight began they follow before going home,
// 0 the world's (entity.DefaultLeash, 300; petition 1535: the West Plateau's
// phantoms went home too soon). see_invis 1: they notice an invisible player
// (petition 1814, Skywalker: the moth larvae), attacking it on sight as any
// other; being seen does not end the invisibility, a hit does (invis.go). By NAME, as factionhits is: a GM's #spawn
// copy shares it. source: gm, or seed for tools/seed_npc_aggro.py's rows (its
// --undo deletes those).
const npcAggroSchema = `
	CREATE TABLE IF NOT EXISTS npcaggro (
		npc_name   TEXT PRIMARY KEY,
		aggressive INTEGER NOT NULL DEFAULT 1,
		radius     REAL NOT NULL DEFAULT 0,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm',
		leash      REAL NOT NULL DEFAULT 0
	);`

// ensureNPCAggro makes table npcaggro (initSchema), adding leash to an older
// one (0: the world's leash, as every creature had) and see_invis (0: blind to
// invisibility, as every creature was).
func (db *SQLiteDB) ensureNPCAggro() error {
	if _, err := db.Exec(npcAggroSchema); err != nil {
		return fmt.Errorf("npcaggro: %w", err)
	}
	has, err := hasColumn(db.DB, "npcaggro", "leash")
	if err != nil {
		return fmt.Errorf("npcaggro: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npcaggro ADD COLUMN leash REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("npcaggro: %w", err)
		}
	}
	has, err = hasColumn(db.DB, "npcaggro", "see_invis")
	if err != nil {
		return fmt.Errorf("npcaggro: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npcaggro ADD COLUMN see_invis INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("npcaggro see_invis: %w", err)
		}
	}
	return nil
}

// NPCAggroRule is one row of npcaggro. NoWord: aggressive below 0, the row
// says nothing of aggro (its leash alone).
type NPCAggroRule struct {
	Name       string
	Aggressive bool
	NoWord     bool
	Radius     float64
	Leash      float64
	// Holding (column holding, questwalls.go's ensure): when not 0 the rule
	// is for the players holding that item alone: they are attacked on sight
	// (or not, by aggressive), every other player is left alone (the Estate's
	// Guardian and Cazic's Scent: "will become KoS and attack you if you have
	// Cazic's Scent on you", t196)
	Holding int
	// SeeInvis (column see_invis): they notice an invisible player (petition 1814)
	SeeInvis bool
}

// NPCAggroRules is every row of npcaggro.
func (db *SQLiteDB) NPCAggroRules() ([]NPCAggroRule, error) {
	rows, err := db.Query(`SELECT npc_name, aggressive, radius, leash, holding, see_invis FROM npcaggro`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCAggroRule
	for rows.Next() {
		var r NPCAggroRule
		var aggressive, seeInvis int
		if err := rows.Scan(&r.Name, &aggressive, &r.Radius, &r.Leash, &r.Holding, &seeInvis); err != nil {
			return nil, err
		}
		r.Aggressive = aggressive > 0
		r.NoWord = aggressive < 0
		r.SeeInvis = seeInvis != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
