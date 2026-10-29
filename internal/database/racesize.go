package database

import (
	"fmt"
	"math"
)

// racesize: a player race's body size on every client (the state channel's
// ModelSize, bytes 36-39), by race on the network enum (Human 0 ... Ogre 9,
// Froglok 10). 1.0 is the race's own body as CHAR.ESF has it, which
// Sandstorm sends for every player (claude-notes/f3-pass-20261025e.md); a
// race with no row is 1.0. Made with jib's gnomes at 0.9 (F3
// 20261029-212125-e7ff987a: "Shrink player gnome size by about 10%"), once,
// when the table is made: a GM's later change or delete of that row stands.
// NPCs keep their own rows' sizes (entity.NPCSize). source: gm, or seed for
// the row made with the table.
const raceSizeSchema = `
	CREATE TABLE IF NOT EXISTS racesize (
		race   INTEGER PRIMARY KEY,
		size   REAL NOT NULL DEFAULT 1,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// The row the table is made with.
const (
	raceSizeSeedRace = 3 // Gnome
	raceSizeSeedSize = 0.9
	raceSizeSeedNote = "jib's F3 20261029-212125: \"Shrink player gnome size by about 10%\""
)

// RaceSizeMin and RaceSizeMax bound a row's size: outside them (or not a
// number) a race is drawn at 1.0, and the GM panel refuses the row.
const (
	RaceSizeMin = 0.25
	RaceSizeMax = 4
)

// ValidRaceSize reports whether size is a player body's size the server
// sends: within [RaceSizeMin, RaceSizeMax].
func ValidRaceSize(size float64) bool {
	return !math.IsNaN(size) && size >= RaceSizeMin && size <= RaceSizeMax
}

// ensureRaceSize makes table racesize (initSchema), with the gnomes' row
// when the table is new.
func (db *SQLiteDB) ensureRaceSize() error {
	var had int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'racesize'`).Scan(&had); err != nil {
		return fmt.Errorf("racesize: %w", err)
	}
	if _, err := db.Exec(raceSizeSchema); err != nil {
		return fmt.Errorf("racesize: %w", err)
	}
	if had == 0 {
		if _, err := db.Exec(`INSERT OR IGNORE INTO racesize (race, size, note, source) VALUES (?, ?, ?, 'seed')`,
			raceSizeSeedRace, raceSizeSeedSize, raceSizeSeedNote); err != nil {
			return fmt.Errorf("racesize: the gnomes' row: %w", err)
		}
	}
	return nil
}

// RaceSizes is every row of racesize: each race's size as written (bounds
// are the reader's: ValidRaceSize).
func (db *SQLiteDB) RaceSizes() (map[int]float64, error) {
	rows, err := db.Query(`SELECT race, size FROM racesize`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]float64)
	for rows.Next() {
		var race int
		var size float64
		if err := rows.Scan(&race, &size); err != nil {
			return nil, err
		}
		out[race] = size
	}
	return out, rows.Err()
}
