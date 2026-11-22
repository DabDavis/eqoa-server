package database

import "fmt"

// itemspeed: a worn item's own run speed (a GM, 2026-11-12, work board 1258:
// a sash "7% faster, beside any other speed"). pct is a percent of the base
// run speed added while the item is worn, beside the movement buff and the
// masteries (entity/itemspeed.go). Read at the start and after a GM's edit; a
// wearer's speed follows at its next change of gear.
const itemSpeedSchema = `
	CREATE TABLE IF NOT EXISTS itemspeed (
		patternid INTEGER PRIMARY KEY,
		pct       REAL NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureItemSpeed makes table itemspeed (initSchema).
func (db *SQLiteDB) ensureItemSpeed() error {
	if _, err := db.Exec(itemSpeedSchema); err != nil {
		return fmt.Errorf("itemspeed: %w", err)
	}
	return nil
}

// ItemSpeeds is table itemspeed: each record's percent of the base run.
func (db *SQLiteDB) ItemSpeeds() (map[int]float64, error) {
	rows, err := db.Query(`SELECT patternid, pct FROM itemspeed`)
	if err != nil {
		return nil, fmt.Errorf("itemspeed: %w", err)
	}
	defer rows.Close()
	out := map[int]float64{}
	for rows.Next() {
		var id int
		var pct float64
		if err := rows.Scan(&id, &pct); err != nil {
			return nil, fmt.Errorf("itemspeed: %w", err)
		}
		out[id] = pct
	}
	return out, rows.Err()
}
