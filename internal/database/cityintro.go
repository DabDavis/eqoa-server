package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// cityintro: a new character's first-login welcome (handlers/cityintro.go),
// one row a start place and class, as the live game's tapes show it: boxes
// (its city and guild, a line a box) said at the first world entry, then the
// shared question "do you need instructions on how to use the basic
// controls?", its Yes going through the shared control boxes to yes_last and
// its No to no_last (each naming the guildmaster). A character matches a row
// when it stands within radius of (x, z) in world at that entry and is of its
// class (and race, unless race is -1).
//
// charintro: what a character was shown of it, a row a character: welcomed
// (UTC, RFC 3339) is when its welcome went out, merchant when the merchant
// tutorial question was asked (once, at its first merchant). Only a character
// that was welcomed is asked the merchant question.
const cityIntroSchema = `
	CREATE TABLE IF NOT EXISTS cityintro (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		world    INTEGER NOT NULL DEFAULT 0,
		x        REAL NOT NULL DEFAULT 0,
		z        REAL NOT NULL DEFAULT 0,
		radius   REAL NOT NULL DEFAULT 60,
		race     INTEGER NOT NULL DEFAULT -1,
		class    INTEGER NOT NULL DEFAULT -1,
		boxes    TEXT NOT NULL DEFAULT '',
		yes_last TEXT NOT NULL DEFAULT '',
		no_last  TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm',
		note     TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS charintro (
		serverid INTEGER PRIMARY KEY,
		welcomed TEXT NOT NULL DEFAULT '',
		merchant TEXT NOT NULL DEFAULT ''
	);`

// ensureCityIntro makes tables cityintro and charintro (initSchema).
func (db *SQLiteDB) ensureCityIntro() error {
	if _, err := db.Exec(cityIntroSchema); err != nil {
		return fmt.Errorf("cityintro: %w", err)
	}
	return nil
}

// CityIntro is a row of cityintro.
type CityIntro struct {
	ID          int
	World       int
	X, Z        float64
	Radius      float64
	Race, Class int
	Boxes       []string // a box a line
	YesLast     string
	NoLast      string
}

// IntroLines splits a column of lines (a box a line), blank lines left out.
func IntroLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// CityIntroFor is the welcome a character of race and class standing at (x,
// z) in world is given: the nearest row whose circle holds it, its race and
// class matched (-1 any; a row naming the class goes before one that does
// not). ok false: none.
func (db *SQLiteDB) CityIntroFor(world, race, class int, x, z float64) (CityIntro, bool, error) {
	rows, err := db.Query(`SELECT id, world, x, z, radius, race, class, boxes, yes_last, no_last FROM cityintro
		WHERE world = ? AND (race = -1 OR race = ?) AND (class = -1 OR class = ?)`, world, race, class)
	if err != nil {
		return CityIntro{}, false, err
	}
	defer rows.Close()
	var best CityIntro
	var bestD float64
	found := false
	for rows.Next() {
		var c CityIntro
		var boxes string
		if err := rows.Scan(&c.ID, &c.World, &c.X, &c.Z, &c.Radius, &c.Race, &c.Class, &boxes, &c.YesLast, &c.NoLast); err != nil {
			return CityIntro{}, false, err
		}
		dx, dz := c.X-x, c.Z-z
		d := dx*dx + dz*dz
		if d > c.Radius*c.Radius {
			continue
		}
		c.Boxes = IntroLines(boxes)
		better := !found || (c.Class >= 0 && best.Class < 0) || (c.Class >= 0) == (best.Class >= 0) && d < bestD
		if better {
			best, bestD, found = c, d, true
		}
	}
	return best, found, rows.Err()
}

// CharIntro is a character's row of charintro: whether it was welcomed and
// asked the merchant question.
type CharIntro struct {
	Welcomed bool
	Merchant bool
}

// CharIntroOf reads the character's row (none: neither).
func (db *SQLiteDB) CharIntroOf(serverID uint32) (CharIntro, error) {
	var w, m string
	err := db.QueryRow(`SELECT welcomed, merchant FROM charintro WHERE serverid = ?`, serverID).Scan(&w, &m)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CharIntro{}, nil
		}
		return CharIntro{}, err
	}
	return CharIntro{Welcomed: w != "", Merchant: m != ""}, nil
}

// TakeIntroWelcome marks the character welcomed, once: it reports whether it
// was not yet (so two sessions of one character cannot both take it).
func (db *SQLiteDB) TakeIntroWelcome(serverID uint32) (bool, error) {
	res, err := db.Exec(`INSERT INTO charintro (serverid, welcomed) VALUES (?, ?)
		ON CONFLICT(serverid) DO UPDATE SET welcomed = excluded.welcomed WHERE charintro.welcomed = ''`,
		serverID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// TakeIntroMerchant marks the merchant question asked of a welcomed
// character, once: it reports whether it was due (welcomed, not yet asked).
func (db *SQLiteDB) TakeIntroMerchant(serverID uint32) (bool, error) {
	res, err := db.Exec(`UPDATE charintro SET merchant = ? WHERE serverid = ? AND welcomed <> '' AND merchant = ''`,
		time.Now().UTC().Format(time.RFC3339), serverID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
