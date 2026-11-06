package database

import "fmt"

// heirlooms: test gear whose figures follow its wearer (the user, 2026-11-06:
// "test armor vendors for starting cities armor that scales per level,
// similar to wow heirloom armor. it will be Best in Slot for each class").
// A row names an item record (itempattern) as one class's heirloom of a role
// (handlers/heirloom.go lists the roles: head, chest, ... ring, main,
// twohand, ranged); the record keeps its own name, look and wear slot, and
// its figures are the best of its role the wearer's class may wear at the
// wearer's level, worked out as each character's view of the record is read
// (PatternView). tools/seed_heirlooms.py writes the records, these rows and
// the vendors who give them.

// HeirloomsSchema is the table.
const HeirloomsSchema = `CREATE TABLE IF NOT EXISTS heirlooms (
	patternid INTEGER PRIMARY KEY,
	class     INTEGER NOT NULL,
	role      TEXT NOT NULL
)`

// HeirloomDef is a record's row: whose heirloom it is and of what role.
type HeirloomDef struct {
	PatternID int
	Class     int
	Role      string
}

// ensureHeirlooms makes the table (initSchema).
func (db *SQLiteDB) ensureHeirlooms() error {
	if _, err := db.Exec(HeirloomsSchema); err != nil {
		return fmt.Errorf("heirlooms: %w", err)
	}
	return nil
}

// Heirlooms is every row, by patternid.
func (db *SQLiteDB) Heirlooms() (map[int]HeirloomDef, error) {
	rows, err := db.Query(`SELECT patternid, class, role FROM heirlooms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]HeirloomDef{}
	for rows.Next() {
		var d HeirloomDef
		if err := rows.Scan(&d.PatternID, &d.Class, &d.Role); err != nil {
			return nil, err
		}
		out[d.PatternID] = d
	}
	return out, rows.Err()
}

// PatternView is a character's own view of an item record, applied to every
// record of a character's items as they are read (GetCharacterItems,
// GetCharItem) and to a merchant's catalog for its buyer (ViewPattern): an
// heirloom's figures are its wearer's (handlers/heirloom.go). It must not
// query through a transaction the caller holds.
type PatternView func(serverID uint32, p *ItemPatternRecord)

// SetPatternView sets the view every store over this database applies; nil
// takes it away.
func (db *SQLiteDB) SetPatternView(v PatternView) {
	db.view.Store(&v)
}

// ViewPattern is the character's view of p (p unchanged without a view).
func (db *SQLiteDB) ViewPattern(serverID uint32, p *ItemPatternRecord) {
	if v := db.view.Load(); v != nil && *v != nil {
		(*v)(serverID, p)
	}
}

// ViewPattern is the character's view of p, as SQLiteDB.ViewPattern.
func (s *InventoryStore) ViewPattern(serverID uint32, p *ItemPatternRecord) {
	if s != nil && s.db != nil {
		s.db.ViewPattern(serverID, p)
	}
}
