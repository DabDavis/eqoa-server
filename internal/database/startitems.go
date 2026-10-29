package database

import (
	"fmt"
	"log"
)

// startItemsSchema is what a new character carries (the user, 2026-10-29: "we
// need starting weapons, sandstorm has it correct"): Sandstorm's new
// characters each carry a starting weapon (an Apprentice Sword, Club, Dagger
// or Staff) and twenty of their race's rations, by race and class (and, for
// a human, its city: humtype 1 the east, 2 the west), none worn; recorded by
// eqoa-rust-headless's faction survey, a new character of every combo, and
// written by tools/seed_start_items.py. A row an item of a combo; count is
// its stack. Read at every creation (CreateNew), so a GM's change is live.
const startItemsSchema = `
CREATE TABLE IF NOT EXISTS startitems (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	race      INTEGER NOT NULL,
	class     INTEGER NOT NULL,
	humtype   INTEGER NOT NULL DEFAULT 0,
	patternid INTEGER NOT NULL,
	count     INTEGER NOT NULL DEFAULT 1,
	note      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT 'gm'
);
CREATE INDEX IF NOT EXISTS idx_startitems_combo ON startitems(race, class, humtype);`

func (db *SQLiteDB) ensureStartItems() error {
	if _, err := db.Exec(startItemsSchema); err != nil {
		return fmt.Errorf("startitems: %w", err)
	}
	return nil
}

// StartItem is one of startitems' rows: an item a new character carries and
// how many.
type StartItem struct {
	PatternID, Count int
}

// StartItems is what a new character of the race, class and humType carries
// (a froglok an eastern human's, as it starts where one does: startPlace).
func (db *SQLiteDB) StartItems(race, class, humType int) ([]StartItem, error) {
	if race == raceFroglok {
		race, humType = 0, 1
	}
	if race != 0 {
		humType = 0 // only a human's city counts
	}
	rows, err := db.Query(`SELECT patternid, count FROM startitems WHERE race = ? AND class = ? AND humtype = ? ORDER BY id`,
		race, class, humType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StartItem
	for rows.Next() {
		var it StartItem
		if err := rows.Scan(&it.PatternID, &it.Count); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// giveStartItems puts a new character's starting items in its inventory,
// carried as Sandstorm's are (none worn), each at its record's full condition
// with no charges (Sandstorm's Apprentice Sword: 200 of 200); an item the
// database lacks is said and left out.
func (s *SQLiteCharacterStore) giveStartItems(c *CharacterRecord) (int, error) {
	items, err := s.db.StartItems(c.Race, c.TClass, c.HumType)
	if err != nil {
		return 0, err
	}
	inv := &InventoryStore{db: s.db}
	n := 0
	for _, it := range items {
		var hp int
		if err := s.db.QueryRow(`SELECT maxhp FROM itempattern WHERE patternid = ?`, it.PatternID).Scan(&hp); err != nil {
			log.Printf("CreateNew: %q's starting item %d: %v (left out)", c.CharName, it.PatternID, err)
			continue
		}
		if _, err := inv.AddCharItem(c.ServerID, it.PatternID, max(it.Count, 1), hp, 0, 0); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
