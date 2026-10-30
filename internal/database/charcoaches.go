package database

import (
	"fmt"
	"time"
)

// charcoaches: the stables' ledgers, the coach towns each character has
// signed (handlers/coachledger.go; the user, 2026-10-30: "make it unlocked by
// talking to the coachman and also only coach to connecting"). Talking to a
// town's coachman signs his ledger, and a coachman's menu offers only the
// towns his coach runs to (entity/coachnetwork.go) whose ledgers the
// character has signed; a GM's offers every coach town. town is the name the
// coach menus list it by (entity/coachtowns.go: Freeport, Klik'Anon, Hazinak
// (Evil)); source talk (signed in the game) or gm (the GM panel's grant);
// signed when, RFC 3339 in UTC (empty for a grant that left it out). No row, no
// town: a character made before the ledgers signs them as it talks.
const charCoachesSchema = `
	CREATE TABLE IF NOT EXISTS charcoaches (
		serverid INTEGER NOT NULL,
		town     TEXT NOT NULL,
		source   TEXT NOT NULL DEFAULT 'gm',
		signed   TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serverid, town)
	);`

// ensureCharCoaches makes table charcoaches (initSchema).
func (db *SQLiteDB) ensureCharCoaches() error {
	if _, err := db.Exec(charCoachesSchema); err != nil {
		return fmt.Errorf("charcoaches: %w", err)
	}
	return nil
}

// CoachTowns is the coach towns whose ledger the character has signed.
func (s *SQLiteCharacterStore) CoachTowns(serverID uint32) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT town FROM charcoaches WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var town string
		if err := rows.Scan(&town); err != nil {
			return nil, err
		}
		out[town] = true
	}
	return out, rows.Err()
}

// SignCoachLedger signs the town's ledger for the character, at; signed is
// false when it already had (the row is left as it was).
func (s *SQLiteCharacterStore) SignCoachLedger(serverID uint32, town string, at time.Time) (signed bool, err error) {
	res, err := s.db.Exec(`INSERT INTO charcoaches (serverid, town, source, signed) VALUES (?, ?, 'talk', ?)
		ON CONFLICT (serverid, town) DO NOTHING`, serverID, town, at.UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
