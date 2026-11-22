package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// npccoins.go — the tunar a named creature's corpse holds, by its name as npcclass keys it (work board 658):
// a roll from coins_min to coins_max instead of the ordinary amount (handlers/corpsecoins.go). Rows of source
// video-audit are amounts read off live-era footage. Read at each corpse's first roll: a GM's edit counts for
// the next corpse.
const npcCoinsSchema = `
	CREATE TABLE IF NOT EXISTS npccoins (
		npc_name  TEXT PRIMARY KEY,
		coins_min INTEGER NOT NULL,
		coins_max INTEGER NOT NULL,
		note      TEXT,
		source    TEXT
	);`

// ensureNPCCoins makes table npccoins (initSchema).
func (db *SQLiteDB) ensureNPCCoins() error {
	if _, err := db.Exec(npcCoinsSchema); err != nil {
		return fmt.Errorf("npccoins: %w", err)
	}
	return nil
}

// NPCCoins is the named amount a creature of that name carries (ok false: no row, the ordinary rule). A row
// with min above max, or below 0, is read as its bounds put right (the panel refuses such a row).
func (s *InventoryStore) NPCCoins(name string) (lo, hi int, ok bool, err error) {
	if s == nil || s.db == nil {
		return 0, 0, false, nil
	}
	err = s.db.QueryRow(`SELECT coins_min, coins_max FROM npccoins WHERE npc_name = ?`, NPCClassKey(name)).Scan(&lo, &hi)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	lo = max(lo, 0)
	hi = max(hi, lo)
	return lo, hi, true, nil
}
