package database

import "fmt"

// loottables and npcloot: what a creature's corpse rolls at its death, beside
// its npcitem rows (handlers/loot.go; F3 pass 18, the user's "if you see the
// loot tables for them add them"). loottables is the tables' items, a row an
// item of a table: tablename (an EQOA-Data biome's, "generic", a named
// creature's own), the item, its weight among the table's, and the creature
// levels it drops for (minlevel..maxlevel). npcloot binds a creature by NAME,
// as npcclass keys it (NPCClassKey), to a table: chance is a roll's, in
// percent, rolls how many a kill makes; each roll that lands takes one item
// of the table for the creature's level, by weight. source: gm, or
// camp-seed for tools/seed_camp_loot.py's rows (its --undo deletes those it
// wrote that no GM has changed).
const lootSchema = `
	CREATE TABLE IF NOT EXISTS loottables (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		tablename TEXT NOT NULL,
		patternid INTEGER NOT NULL,
		weight    INTEGER NOT NULL DEFAULT 1,
		minlevel  INTEGER NOT NULL DEFAULT 1,
		maxlevel  INTEGER NOT NULL DEFAULT 60,
		source    TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE INDEX IF NOT EXISTS idx_loottables_table ON loottables(tablename);
	CREATE TABLE IF NOT EXISTS npcloot (
		npc_name  TEXT NOT NULL,
		tablename TEXT NOT NULL,
		chance    INTEGER NOT NULL DEFAULT 25,
		rolls     INTEGER NOT NULL DEFAULT 1,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (npc_name, tablename)
	);`

// ensureLootTables makes tables loottables and npcloot, and the categories'
// lootcategory, lootpool and lootregion (lootcategory.go) (initSchema).
func (db *SQLiteDB) ensureLootTables() error {
	if _, err := db.Exec(lootSchema); err != nil {
		return fmt.Errorf("loottables: %w", err)
	}
	if _, err := db.Exec(lootCategorySchema); err != nil {
		return fmt.Errorf("lootcategory: %w", err)
	}
	return nil
}

// LootChoice is one item a table may give a creature: the corpse's record of
// it (one, at full condition) and its weight among the table's.
type LootChoice struct {
	Item   CharInventoryEntry
	Weight int
}

// LootRoll is one table bound to a creature: its chance a roll (percent),
// its rolls a kill, and its items for the creature's level.
type LootRoll struct {
	Table   string
	Chance  int
	Rolls   int
	Choices []LootChoice
}

// NPCLootRolls is the loot tables bound to a creature of that name, each with
// its items for that level (none: the table gives nothing at this level).
func (s *InventoryStore) NPCLootRolls(name string, level int) ([]LootRoll, error) {
	// every binding first, the rows closed: the database has one connection
	rows, err := s.db.Query(`SELECT tablename, chance, rolls FROM npcloot WHERE npc_name = ? ORDER BY tablename`,
		NPCClassKey(name))
	if err != nil {
		return nil, err
	}
	var out []LootRoll
	for rows.Next() {
		var r LootRoll
		if err := rows.Scan(&r.Table, &r.Chance, &r.Rolls); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		choices, err := s.lootChoices(out[i].Table, level)
		if err != nil {
			return nil, err
		}
		out[i].Choices = choices
	}
	return out, nil
}

// lootChoices is a table's items for a creature of that level.
func (s *InventoryStore) lootChoices(table string, level int) ([]LootChoice, error) {
	rows, err := s.db.Query(`
		SELECT 0, 1, p.maxhp, 0, -1, 0, 0,
		`+itemPatternColumns+`, t.weight
		FROM loottables t
		JOIN itempattern p ON t.patternid = p.patternid
		WHERE t.tablename = ? AND t.minlevel <= ? AND t.maxlevel >= ? AND t.weight > 0
		ORDER BY t.id
	`, table, level, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LootChoice
	for rows.Next() {
		var c LootChoice
		e, err := scanItemRow(func(dest ...any) error { return rows.Scan(append(dest, &c.Weight)...) })
		if err != nil {
			return nil, err
		}
		c.Item = e
		out = append(out, c)
	}
	return out, rows.Err()
}
