package database

import (
	"fmt"
	"strings"
)

// itemfactions: an item whose Use raises the user's standing with factions
// (handlers/itemfactions.go). Petition 5089 (Live, Trulyblind, 2026-11-15:
// "would it be possible to get some kind of item or something, so I can test
// the other 17s?"); the owner: "give them faction stone that increases
// factions with all lvl 17 q givers". A row is one faction the item raises:
// its Use brings the user's standing with faction up to standing (never down:
// a standing already at or past it stays), once a use; the item is not spent.
// note and source say who wrote the row and why.
const itemFactionsSchema = `
	CREATE TABLE IF NOT EXISTS itemfactions (
		patternid INTEGER NOT NULL,
		faction   TEXT NOT NULL,
		standing  INTEGER NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (patternid, faction)
	);`

// ensureItemFactions makes table itemfactions (initSchema).
func (db *SQLiteDB) ensureItemFactions() error {
	if _, err := db.Exec(itemFactionsSchema); err != nil {
		return fmt.Errorf("itemfactions: %w", err)
	}
	return nil
}

// ItemFaction is one row of itemfactions: the Use raises Faction to Standing.
type ItemFaction struct {
	Faction  string
	Standing int
}

// ItemFactionsOf is the factions an item's Use raises, by faction; none when
// it has no rows.
func (s *InventoryStore) ItemFactionsOf(patternID int) ([]ItemFaction, error) {
	rows, err := s.db.Query(`SELECT faction, standing FROM itemfactions WHERE patternid = ? ORDER BY faction`, patternID)
	if err != nil {
		return nil, fmt.Errorf("itemfactions: %w", err)
	}
	defer rows.Close()
	var out []ItemFaction
	for rows.Next() {
		var f ItemFaction
		if err := rows.Scan(&f.Faction, &f.Standing); err != nil {
			return nil, fmt.Errorf("itemfactions: %w", err)
		}
		if f.Faction = strings.TrimSpace(f.Faction); f.Faction != "" {
			out = append(out, f)
		}
	}
	return out, rows.Err()
}
