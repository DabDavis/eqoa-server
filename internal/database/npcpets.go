package database

import "fmt"

// npcpets: the pets a creature summons (the owner, 2026-11-25: "any mob with a pet should summon their own pets so
// dont need any pets placed"; entity/creaturepets.go). By the OWNER: owner is its name as NPCClassKey keys it, or
// "#<row>" for one creature of an npcs row (and the copies of that row) whose pets are its own, ahead of its name's.
// template_row is the npcs row each pet is a copy of (its look, name, faction and own rules; held out of the world by
// an npcspawnonly "#row"), count how many of it, level_offset its level against its owner's (at least 1). The server
// makes them beside the owner at each spawn and takes them away with it; a pet killed alone comes back with its
// owner's next spawn. tools/creature_pets_20261125.py moved the placed pet rows here.
const npcPetsSchema = `
	CREATE TABLE IF NOT EXISTS npcpets (
		owner        TEXT NOT NULL,
		pet          TEXT NOT NULL,
		template_row INTEGER NOT NULL,
		count        INTEGER NOT NULL DEFAULT 1,
		level_offset INTEGER NOT NULL DEFAULT 0,
		note         TEXT NOT NULL DEFAULT '',
		source       TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (owner, template_row)
	);`

// ensureNPCPets makes table npcpets (initSchema).
func (db *SQLiteDB) ensureNPCPets() error {
	if _, err := db.Exec(npcPetsSchema); err != nil {
		return fmt.Errorf("npcpets: %w", err)
	}
	return nil
}

// NPCPetRow is a row of npcpets that counts.
type NPCPetRow struct {
	Owner       string // NPCClassKey of the owner's name, or "#<row>"
	Pet         string
	TemplateRow uint32
	Count       int
	LevelOffset int
}

// NPCPets is every row of npcpets that counts (a template row and a count above 0), its owner keyed as
// NPCClassKey keys a name (a "#row" as it is).
func (db *SQLiteDB) NPCPets() ([]NPCPetRow, error) {
	rows, err := db.Query(`SELECT owner, pet, template_row, count, level_offset FROM npcpets
		WHERE template_row > 0 AND count > 0 ORDER BY owner, template_row`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCPetRow
	for rows.Next() {
		var r NPCPetRow
		if err := rows.Scan(&r.Owner, &r.Pet, &r.TemplateRow, &r.Count, &r.LevelOffset); err != nil {
			return nil, err
		}
		r.Owner = NPCClassKey(r.Owner)
		out = append(out, r)
	}
	return out, rows.Err()
}
