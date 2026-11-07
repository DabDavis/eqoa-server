package database

import (
	"fmt"
	"time"
)

// potioncooldowns: when a character may use an item with a cooldown
// (itemeffect.cooldown, entity/potion.go) again, written at each use and read
// at the login, so neither a logout, a relog nor a restart ends it. until is
// in Unix milliseconds; a row past it is nothing.
const potionCooldownSchema = `
	CREATE TABLE IF NOT EXISTS potioncooldowns (
		serverid  INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		until     INTEGER NOT NULL,
		PRIMARY KEY (serverid, patternid)
	);`

func (db *SQLiteDB) ensurePotionCooldowns() error {
	if _, err := db.Exec(potionCooldownSchema); err != nil {
		return fmt.Errorf("potioncooldowns: %w", err)
	}
	return nil
}

// PotionCooldowns is a character's items still cooling at now, by item: when
// each may be used again.
func (s *InventoryStore) PotionCooldowns(serverID int, now time.Time) (map[int]time.Time, error) {
	rows, err := s.db.Query(`SELECT patternid, until FROM potioncooldowns WHERE serverid = ? AND until > ?`, serverID, now.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]time.Time{}
	for rows.Next() {
		var pid int
		var until int64
		if err := rows.Scan(&pid, &until); err != nil {
			return nil, err
		}
		out[pid] = time.UnixMilli(until)
	}
	return out, rows.Err()
}

// SetPotionCooldown keeps when a character may use an item again.
func (s *InventoryStore) SetPotionCooldown(serverID, patternID int, until time.Time) error {
	_, err := s.db.Exec(`INSERT INTO potioncooldowns (serverid, patternid, until) VALUES (?, ?, ?)
		ON CONFLICT (serverid, patternid) DO UPDATE SET until = excluded.until`, serverID, patternID, until.UnixMilli())
	return err
}
