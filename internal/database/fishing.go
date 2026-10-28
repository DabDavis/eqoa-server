package database

import (
	"fmt"
)

// fishcatch: what a cast can bring up (handlers/fishing.go). One row an
// item: itemid the itempattern it is, count how many of it come up at once,
// weight its share among the rows that apply where and when the line went in.
// A row applies in world (-1: every world), inside the box minx..maxx,
// minz..maxz when the four are given (NULL: anywhere in the world), to an
// angler of fishing skill minskill or more (characters.fishing, 0 to 100),
// in the months of months (a bit a month, bit 0 January; 0 the whole year).
// note says why; source: gm, or seed for tools/seed_fishing.py's rows (its
// --undo deletes those).
const fishCatchSchema = `
	CREATE TABLE IF NOT EXISTS fishcatch (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		itemid   INTEGER NOT NULL,
		count    INTEGER NOT NULL DEFAULT 1,
		weight   INTEGER NOT NULL DEFAULT 10,
		world    INTEGER NOT NULL DEFAULT -1,
		minx     REAL,
		minz     REAL,
		maxx     REAL,
		maxz     REAL,
		minskill INTEGER NOT NULL DEFAULT 0,
		months   INTEGER NOT NULL DEFAULT 0,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// FishCatch is one row of fishcatch.
type FishCatch struct {
	ID       int
	ItemID   int // itempattern.patternid
	Count    int
	Weight   int
	World    int // -1 every world
	Box      bool
	MinX     float32
	MinZ     float32
	MaxX     float32
	MaxZ     float32
	MinSkill int
	Months   int // bit m-1 for month m; 0 the whole year
}

// Applies reports a row that may come up from a cast landing at (x, z) in a
// world, by an angler of that skill, in month (1 January .. 12 December).
func (c FishCatch) Applies(world uint8, x, z float32, skill, month int) bool {
	if c.World >= 0 && c.World != int(world) {
		return false
	}
	if c.Box && (x < c.MinX || x > c.MaxX || z < c.MinZ || z > c.MaxZ) {
		return false
	}
	if skill < c.MinSkill {
		return false
	}
	if c.Months != 0 && month >= 1 && month <= 12 && c.Months&(1<<(month-1)) == 0 {
		return false
	}
	return c.Weight > 0 && c.Count > 0
}

// ensureFishCatch makes table fishcatch (initSchema).
func (db *SQLiteDB) ensureFishCatch() error {
	if _, err := db.Exec(fishCatchSchema); err != nil {
		return fmt.Errorf("fishcatch: %w", err)
	}
	return nil
}

// FishCatches is every row of fishcatch for a world (its own and the rows
// of every world), in id order. A row's box counts only when all four of
// its edges are given.
func (db *SQLiteDB) FishCatches(world uint8) ([]FishCatch, error) {
	rows, err := db.Query(`SELECT id, itemid, count, weight, world, minx, minz, maxx, maxz, minskill, months
		FROM fishcatch WHERE world = -1 OR world = ? ORDER BY id`, int(world))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FishCatch
	for rows.Next() {
		var c FishCatch
		var x0, z0, x1, z1 *float64
		if err := rows.Scan(&c.ID, &c.ItemID, &c.Count, &c.Weight, &c.World, &x0, &z0, &x1, &z1, &c.MinSkill, &c.Months); err != nil {
			return nil, err
		}
		if x0 != nil && z0 != nil && x1 != nil && z1 != nil {
			c.Box = true
			c.MinX, c.MinZ, c.MaxX, c.MaxZ = float32(*x0), float32(*z0), float32(*x1), float32(*z1)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FishingSkill is a character's fishing skill (characters.fishing).
func (s *SQLiteCharacterStore) FishingSkill(serverID uint32) (int, error) {
	var skill int
	err := s.db.QueryRow(`SELECT fishing FROM characters WHERE serverid = ?`, serverID).Scan(&skill)
	return skill, err
}

// SetFishingSkill stores a character's fishing skill, and nothing else of
// the row (the logout save reads the row again before it writes its own).
func (s *SQLiteCharacterStore) SetFishingSkill(serverID uint32, skill int) error {
	res, err := s.db.Exec(`UPDATE characters SET fishing = ? WHERE serverid = ?`, skill, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	return nil
}
