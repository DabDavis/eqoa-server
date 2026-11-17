package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// gmseasons and npcseason: a season a GM turns on and off (the owner,
// 2026-11-16: "lets make a christmas season trigger"), kept over restarts, and
// the NPCs that stand in the world only while their season is on.
//
// A season is a row of gmseasons, active 1 or 0; a season no row names is off.
// A GM trigger's season step turns one on, off or over (handlers/gmtriggers.go),
// and the GM panel's Seasons page edits the rows. An npcseason row names an NPC
// (by name, as npcspawnonly does: NPCClassKey) and its season: while the season
// is off every NPC of the name is held out of the world exactly as a spawn-only
// one is (entity/spawnonly.go: in no view, fighting and helping no one), and
// comes back into every view the moment it turns on. The decorations' lights
// (decorlights.go) name a season too.
const seasonsSchema = `
	CREATE TABLE IF NOT EXISTS gmseasons (
		name    TEXT PRIMARY KEY COLLATE NOCASE,
		active  INTEGER NOT NULL DEFAULT 0,
		changed TEXT NOT NULL DEFAULT '',
		by      TEXT NOT NULL DEFAULT '',
		note    TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS npcseason (
		name   TEXT PRIMARY KEY,
		season TEXT NOT NULL DEFAULT '',
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureSeasons makes tables gmseasons and npcseason (initSchema).
func (db *SQLiteDB) ensureSeasons() error {
	if _, err := db.Exec(seasonsSchema); err != nil {
		return fmt.Errorf("gmseasons: %w", err)
	}
	return nil
}

// Season is a row of gmseasons.
type Season struct {
	Name    string `json:"name"`
	Active  bool   `json:"active"`
	Changed string `json:"changed"`
	By      string `json:"by"`
	Note    string `json:"note"`
}

// SeasonKey is a season's name as the tables match it: any case, trimmed.
func SeasonKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Seasons is every row of gmseasons, by name.
func (db *SQLiteDB) Seasons() ([]Season, error) {
	rows, err := db.Query(`SELECT name, active, changed, by, note FROM gmseasons ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Season
	for rows.Next() {
		var s Season
		var on int
		if err := rows.Scan(&s.Name, &on, &s.Changed, &s.By, &s.Note); err != nil {
			return nil, err
		}
		s.Active = on != 0
		out = append(out, s)
	}
	return out, rows.Err()
}

// ActiveSeasons is the seasons that are on, by SeasonKey.
func (db *SQLiteDB) ActiveSeasons() (map[string]bool, error) {
	all, err := db.Seasons()
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool)
	for _, s := range all {
		if s.Active {
			out[SeasonKey(s.Name)] = true
		}
	}
	return out, nil
}

// SeasonHolds reports whether a row of a season holds while the seasons on
// (ActiveSeasons) are: a row of no season (blank) always, one of a season while
// it is on (npctint.season, npcdyes.season).
func SeasonHolds(season string, on map[string]bool) bool {
	k := SeasonKey(season)
	return k == "" || on[k]
}

// SeasonActive reports whether the season is on (no row: off).
func (db *SQLiteDB) SeasonActive(name string) (bool, error) {
	var on int
	err := db.QueryRow(`SELECT active FROM gmseasons WHERE name = ? COLLATE NOCASE`, strings.TrimSpace(name)).Scan(&on)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return on != 0, nil
}

// SetSeason turns a season on or off (making its row when there is none), by
// whom, and reports whether it changed. The row's name keeps the case it was
// made with.
func (db *SQLiteDB) SetSeason(name string, on bool, by string) (changed bool, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("a season needs a name")
	}
	was, err := db.SeasonActive(name)
	if err != nil {
		return false, err
	}
	v := 0
	if on {
		v = 1
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	_, err = db.Exec(`INSERT INTO gmseasons (name, active, changed, by) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET active = excluded.active, changed = excluded.changed, by = excluded.by`,
		name, v, now, by)
	if err != nil {
		return false, err
	}
	return was != on, nil
}

// NPCSeasonHeld is the NPC names (by NPCClassKey) of npcseason whose season is
// off: held out of the world as spawn-only names are.
func (db *SQLiteDB) NPCSeasonHeld() (map[string]bool, error) {
	active, err := db.ActiveSeasons()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT name, season FROM npcseason`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name, season string
		if err := rows.Scan(&name, &season); err != nil {
			return nil, err
		}
		if k := NPCClassKey(name); k != "" && !active[SeasonKey(season)] {
			out[k] = true
		}
	}
	return out, rows.Err()
}

// NPCSeasonCount is how many npcseason names the season holds.
func (db *SQLiteDB) NPCSeasonCount(season string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT count(*) FROM npcseason WHERE lower(trim(season)) = ?`, SeasonKey(season)).Scan(&n)
	return n, err
}
