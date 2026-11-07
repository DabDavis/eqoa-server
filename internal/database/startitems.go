package database

import (
	"database/sql"
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
// (a froglok an eastern human's, as it starts where one does: startPlace). A
// combo the table lacks (a class its race never had, now that every class may
// be open to every race: entity/creation.go; a race of racedefs with no rows of
// its own, which carries its wire race's) carries its race's items but their
// weapon, and the weapon its class carries most (startItemsFallback).
func (db *SQLiteDB) StartItems(race, class, humType int) ([]StartItem, error) {
	if race == raceFroglok {
		race, humType = 0, 1
	}
	if race != 0 {
		humType = 0 // only a human's city counts
	}
	out, err := db.StartItemsOf(race, class, humType)
	if err != nil || len(out) > 0 {
		return out, err
	}
	return db.startItemsFallback(race, class, humType)
}

// StartItemsOf is startitems' rows for one combo, as they are: none for a
// combo the table lacks (what the balance tools' start kits read).
func (db *SQLiteDB) StartItemsOf(race, class, humType int) ([]StartItem, error) {
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

// startItemsFallback is a combo with no rows: what the race carries in its
// first combo with rows (a human's of its own city first), a weapon (an item of
// an attack type) left out, then the weapon most combos of the class carry (a
// monk none). A race of racedefs with no rows of its own takes its wire race's.
// Nothing at all for a race with no rows anywhere.
func (db *SQLiteDB) startItemsFallback(race, class, humType int) ([]StartItem, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM startitems WHERE race = ?`, race).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 && race > raceFroglok {
		wire, city, ok := db.raceDefHome(race)
		if !ok {
			return nil, nil
		}
		return db.StartItems(wire, class, city)
	}
	var from, fromHum int
	err := db.QueryRow(`SELECT class, humtype FROM startitems WHERE race = ?
		ORDER BY humtype = ? DESC, humtype, class LIMIT 1`, race, humType).Scan(&from, &fromHum)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT s.patternid, s.count FROM startitems s JOIN itempattern p ON p.patternid = s.patternid
		WHERE s.race = ? AND s.class = ? AND s.humtype = ? AND p.attacktype = 0 ORDER BY s.id`, race, from, fromHum)
	if err != nil {
		return nil, err
	}
	var out []StartItem
	for rows.Next() {
		var it StartItem
		if err := rows.Scan(&it.PatternID, &it.Count); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var weapon int
	switch err := db.QueryRow(`SELECT s.patternid FROM startitems s JOIN itempattern p ON p.patternid = s.patternid
		WHERE s.class = ? AND p.attacktype != 0 GROUP BY s.patternid ORDER BY COUNT(*) DESC, s.patternid LIMIT 1`, class).Scan(&weapon); err {
	case nil:
		out = append(out, StartItem{PatternID: weapon, Count: 1})
	case sql.ErrNoRows:
	default:
		return nil, err
	}
	return out, nil
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
