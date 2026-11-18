package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// hardcore.go — [HC] Hardcore characters (the owner, 2026-11-18: "lets add [HC]
// tag for characters. Hardcore mode chars stay dead and cannot respawn"; chosen
// at character creation, permanent; death is final; a fallen character stays
// as a locked memorial until its player deletes it). Columns of characters,
// every one 0 or empty for a character made before them (none is Hardcore):
//
//	hardcore       1: a Hardcore character
//	hc_chosen      1: the choice is made (at creation by a client that offers
//	               it, or in the question box a character's first world entry
//	               asks: handlers/hardcore.go); 0 with hc_born set: still open
//	hc_born        Unix seconds the character was made (0: before this column)
//	hc_born_level  its level then: the box is asked only while it is still at
//	               it with no experience (a choice for a new character alone)
//	hc_played      seconds in the world, kept for a Hardcore character
//	fallen_at      Unix seconds of its fall; 0: not fallen
//	fallen_*       the fall as it happened: what killed it (cause, killer),
//	               where (place, world, x, y, z), and the character then
//	               (level, class, race, cms: its Combat Mastery points earned,
//	               played: hc_played at the fall)
//
// The fall is written once (Fall: only a living Hardcore character falls); a
// GM's unfall (the panel's unfall_character, audited) clears fallen_at and
// keeps the rest in the audit's row.

// hardcoreColumns are the columns ensureHardcore adds to characters.
var hardcoreColumns = []struct{ name, def string }{
	{"hardcore", "INTEGER NOT NULL DEFAULT 0"},
	{"hc_chosen", "INTEGER NOT NULL DEFAULT 0"},
	{"hc_born", "INTEGER NOT NULL DEFAULT 0"},
	{"hc_born_level", "INTEGER NOT NULL DEFAULT 0"},
	{"hc_played", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_at", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_cause", "TEXT NOT NULL DEFAULT ''"},
	{"fallen_killer", "TEXT NOT NULL DEFAULT ''"},
	{"fallen_place", "TEXT NOT NULL DEFAULT ''"},
	{"fallen_world", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_x", "REAL NOT NULL DEFAULT 0"},
	{"fallen_y", "REAL NOT NULL DEFAULT 0"},
	{"fallen_z", "REAL NOT NULL DEFAULT 0"},
	{"fallen_level", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_class", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_race", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_cms", "INTEGER NOT NULL DEFAULT 0"},
	{"fallen_played", "INTEGER NOT NULL DEFAULT 0"},
}

// ensureHardcore adds the Hardcore columns to characters (initSchema).
func (db *SQLiteDB) ensureHardcore() error {
	for _, c := range hardcoreColumns {
		ok, err := hasColumn(db.DB, "characters", c.name)
		if err != nil {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
		if ok {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE characters ADD COLUMN ` + c.name + ` ` + c.def); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("characters.%s: %w", c.name, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS characters_fallen ON characters (fallen_at) WHERE fallen_at > 0`); err != nil {
		return fmt.Errorf("characters_fallen: %w", err)
	}
	return nil
}

// HardcoreState is a character's Hardcore columns.
type HardcoreState struct {
	ServerID  uint32
	Name      string
	Hardcore  bool
	Chosen    bool
	Born      int64 // Unix seconds; 0 unknown
	BornLevel int
	Played    int64 // seconds
	Level     int   // its level now
	TotalXP   int
	Fall      FallRecord // At 0: not fallen
}

// Fallen reports a fallen character.
func (h HardcoreState) Fallen() bool { return h.Fall.At > 0 }

// ChoiceOpen reports a character whose Hardcore choice is still to be asked:
// one made since the columns (hc_born), not chosen, still at the level it was
// made at with no experience.
func (h HardcoreState) ChoiceOpen() bool {
	return !h.Chosen && !h.Hardcore && h.Born > 0 && h.Level == h.BornLevel && h.TotalXP == 0
}

// FallRecord is a fall as it was written.
type FallRecord struct {
	ServerID             uint32
	Name                 string
	At                   int64 // Unix seconds UTC
	Cause, Killer, Place string
	World                int
	X, Y, Z              float64
	Level, Class, Race   int
	CMs                  int
	Played               int64 // seconds in the world, at the fall
	Born                 int64 // Unix seconds the character was made; 0 unknown
	AccountID            int
}

const hardcoreSelect = `SELECT serverid, charName, COALESCE(hardcore, 0), COALESCE(hc_chosen, 0), COALESCE(hc_born, 0),
	COALESCE(hc_born_level, 0), COALESCE(hc_played, 0), level, COALESCE(totalXP, 0), accountid,
	COALESCE(fallen_at, 0), COALESCE(fallen_cause, ''), COALESCE(fallen_killer, ''), COALESCE(fallen_place, ''),
	COALESCE(fallen_world, 0), COALESCE(fallen_x, 0), COALESCE(fallen_y, 0), COALESCE(fallen_z, 0),
	COALESCE(fallen_level, 0), COALESCE(fallen_class, 0), COALESCE(fallen_race, 0), COALESCE(fallen_cms, 0),
	COALESCE(fallen_played, 0) FROM characters`

func scanHardcore(sc interface{ Scan(...any) error }) (HardcoreState, error) {
	var h HardcoreState
	var hc, chosen int
	f := &h.Fall
	err := sc.Scan(&h.ServerID, &h.Name, &hc, &chosen, &h.Born, &h.BornLevel, &h.Played, &h.Level, &h.TotalXP, &f.AccountID,
		&f.At, &f.Cause, &f.Killer, &f.Place, &f.World, &f.X, &f.Y, &f.Z, &f.Level, &f.Class, &f.Race, &f.CMs, &f.Played)
	h.Hardcore, h.Chosen = hc != 0, chosen != 0
	f.ServerID, f.Name, f.Born = h.ServerID, h.Name, h.Born
	return h, err
}

// HardcoreOf is a character's Hardcore columns; ErrCharacterNotFound for none.
func (db *SQLiteDB) HardcoreOf(serverID uint32) (HardcoreState, error) {
	h, err := scanHardcore(db.QueryRow(hardcoreSelect+` WHERE serverid = ?`, serverID))
	if errors.Is(err, sql.ErrNoRows) {
		return HardcoreState{}, ErrCharacterNotFound
	}
	return h, err
}

// HardcoreOfAccount is an account's characters' Hardcore columns by server id.
func (db *SQLiteDB) HardcoreOfAccount(accountID int) (map[uint32]HardcoreState, error) {
	rows, err := db.Query(hardcoreSelect+` WHERE accountid = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]HardcoreState{}
	for rows.Next() {
		h, err := scanHardcore(rows)
		if err != nil {
			return nil, err
		}
		out[h.ServerID] = h
	}
	return out, rows.Err()
}

// StampBorn marks a character just made: its birth and level, its choice open
// (CreateNew).
func (db *SQLiteDB) StampBorn(serverID uint32, level int, at time.Time) error {
	_, err := db.Exec(`UPDATE characters SET hc_born = ?, hc_born_level = ? WHERE serverid = ?`, at.Unix(), level, serverID)
	return err
}

// ChooseHardcore makes a character's choice: Hardcore (on) or not. Only an open
// choice is taken (never one made, never a fallen character's): changed
// reports whether it was.
func (db *SQLiteDB) ChooseHardcore(serverID uint32, on bool) (changed bool, err error) {
	v := 0
	if on {
		v = 1
	}
	res, err := db.Exec(`UPDATE characters SET hardcore = ?, hc_chosen = 1
		WHERE serverid = ? AND COALESCE(hc_chosen, 0) = 0 AND COALESCE(hardcore, 0) = 0 AND COALESCE(fallen_at, 0) = 0`, v, serverID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddHardcorePlayed adds seconds in the world to a Hardcore character's count.
func (db *SQLiteDB) AddHardcorePlayed(serverID uint32, secs int64) error {
	if secs <= 0 {
		return nil
	}
	_, err := db.Exec(`UPDATE characters SET hc_played = COALESCE(hc_played, 0) + ? WHERE serverid = ? AND hardcore = 1
		AND COALESCE(fallen_at, 0) = 0`, secs, serverID)
	return err
}

// Fall writes a Hardcore character's fall, once: only a Hardcore character not
// fallen falls. f.Played is added to hc_played (the session's time since its
// entry) and the sum kept as fallen_played. fell reports whether it was
// written.
func (db *SQLiteDB) Fall(f FallRecord) (fell bool, err error) {
	res, err := db.Exec(`UPDATE characters SET fallen_at = ?, fallen_cause = ?, fallen_killer = ?, fallen_place = ?,
		fallen_world = ?, fallen_x = ?, fallen_y = ?, fallen_z = ?, fallen_level = ?, fallen_class = ?, fallen_race = ?,
		fallen_cms = ?, hc_played = COALESCE(hc_played, 0) + ?, fallen_played = COALESCE(hc_played, 0) + ?, currentHP = 0
		WHERE serverid = ? AND hardcore = 1 AND COALESCE(fallen_at, 0) = 0`,
		f.At, f.Cause, f.Killer, f.Place, f.World, f.X, f.Y, f.Z, f.Level, f.Class, f.Race, f.CMs, max(f.Played, 0),
		max(f.Played, 0), f.ServerID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// FallenIDs is every fallen Hardcore character's serverid (fallen_at set): a
// guild never hands its lead to one (entity/guildfall.go).
func (db *SQLiteDB) FallenIDs() ([]uint32, error) {
	rows, err := db.Query(`SELECT serverid FROM characters WHERE COALESCE(fallen_at, 0) > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint32
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FallenList is the shard's fallen characters, the newest fall first, at most
// limit (0: all).
func (db *SQLiteDB) FallenList(limit int) ([]FallRecord, error) {
	return fallenList(db.DB, limit)
}

// FallenListOf is FallenList on any database (another shard's, opened
// read-only): a database without the columns has none.
func FallenListOf(q *sql.DB, limit int) ([]FallRecord, error) {
	if ok, err := hasColumn(q, "characters", "fallen_played"); err != nil || !ok {
		return nil, err
	}
	return fallenList(q, limit)
}

func fallenList(q *sql.DB, limit int) ([]FallRecord, error) {
	query := hardcoreSelect + ` WHERE hardcore = 1 AND fallen_at > 0 ORDER BY fallen_at DESC, serverid DESC`
	var args []any
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FallRecord
	for rows.Next() {
		h, err := scanHardcore(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h.Fall)
	}
	return out, rows.Err()
}

// LivingHero is a living Hardcore character as the leaderboard ranks it.
type LivingHero struct {
	ServerID    uint32
	Name        string
	Level       int
	Class, Race int
	CMs         int
	TotalXP     int64
	Born        int64 // Unix seconds; 0 unknown
	Played      int64 // seconds, at its last logout
}

// LivingHardcore is the shard's living Hardcore characters ranked: by level,
// then Combat Mastery points earned (charmastery's points + spent), then total
// experience, then the longest alive (the earliest made); at most limit (0: all).
func (db *SQLiteDB) LivingHardcore(limit int) ([]LivingHero, error) {
	return livingHardcore(db.DB, limit)
}

// LivingHardcoreOf is LivingHardcore on any database (another shard's).
func LivingHardcoreOf(q *sql.DB, limit int) ([]LivingHero, error) {
	if ok, err := hasColumn(q, "characters", "fallen_played"); err != nil || !ok {
		return nil, err
	}
	return livingHardcore(q, limit)
}

func livingHardcore(q *sql.DB, limit int) ([]LivingHero, error) {
	query := `SELECT c.serverid, c.charName, c.level, c.tclass, c.race, COALESCE(m.points, 0) + COALESCE(m.spent, 0),
		COALESCE(c.totalXP, 0), COALESCE(c.hc_born, 0), COALESCE(c.hc_played, 0)
		FROM characters c LEFT JOIN charmastery m ON m.serverid = c.serverid
		WHERE c.hardcore = 1 AND COALESCE(c.fallen_at, 0) = 0
		ORDER BY c.level DESC, 6 DESC, 7 DESC, CASE WHEN COALESCE(c.hc_born, 0) = 0 THEN 1 ELSE 0 END, c.hc_born ASC, c.serverid ASC`
	var args []any
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LivingHero
	for rows.Next() {
		var h LivingHero
		if err := rows.Scan(&h.ServerID, &h.Name, &h.Level, &h.Class, &h.Race, &h.CMs, &h.TotalXP, &h.Born, &h.Played); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
