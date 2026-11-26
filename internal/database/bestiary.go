package database

import (
	"fmt"
	"strings"
	"time"
)

// charbestiary: each character's bestiary (Heirloom petition 3192: "some type of
// bestiary that shows mobs you encounter and the zones they are from";
// entity/bestiary.go decides what counts, handlers/bestiary.go shows it). A row a
// creature kind (the NPC's name) and the zone it was met in: when it was first met
// (fought: it held the character's hate, or the character's pet's) and how many of it
// the character was credited with killing, the last when. A row is made at the first
// meeting and only counted up, so it survives logouts and restarts as any row does;
// nothing before this table's first deploy is in it. Each shard keeps its own.
const charBestiarySchema = `
	CREATE TABLE IF NOT EXISTS charbestiary (
		serverid   INTEGER NOT NULL,
		kind       TEXT NOT NULL COLLATE NOCASE,
		zone       TEXT NOT NULL COLLATE NOCASE,
		first_seen INTEGER NOT NULL DEFAULT 0,
		kills      INTEGER NOT NULL DEFAULT 0,
		last_kill  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (serverid, kind, zone)
	);`

// ensureCharBestiary makes table charbestiary (initSchema).
func (db *SQLiteDB) ensureCharBestiary() error {
	if _, err := db.Exec(charBestiarySchema); err != nil {
		return fmt.Errorf("charbestiary: %w", err)
	}
	return nil
}

// BestiaryNote is one thing the bestiary is told: a character met a kind in a zone
// at At, and, with Kill, was credited with killing one.
type BestiaryNote struct {
	ServerID uint32
	Kind     string
	Zone     string
	At       time.Time
	Kill     bool
}

// BestiaryEntry is a row of a character's bestiary.
type BestiaryEntry struct {
	Kind      string
	Zone      string
	FirstSeen time.Time
	Kills     int
	LastKill  time.Time // zero for none
}

// RecordBestiary writes notes in one transaction: each a row made at its first
// meeting (its time the first seen), a kill counted up on it.
func (db *SQLiteDB) RecordBestiary(notes []BestiaryNote) error {
	if len(notes) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, n := range notes {
		kind, zone := strings.TrimSpace(n.Kind), strings.TrimSpace(n.Zone)
		if n.ServerID == 0 || kind == "" {
			continue
		}
		at := n.At.Unix()
		if _, err := tx.Exec(`INSERT OR IGNORE INTO charbestiary (serverid, kind, zone, first_seen) VALUES (?, ?, ?, ?)`,
			n.ServerID, kind, zone, at); err != nil {
			return fmt.Errorf("charbestiary: %w", err)
		}
		if n.Kill {
			if _, err := tx.Exec(`UPDATE charbestiary SET kills = kills + 1, last_kill = ? WHERE serverid = ? AND kind = ? AND zone = ?`,
				at, n.ServerID, kind, zone); err != nil {
				return fmt.Errorf("charbestiary: %w", err)
			}
		}
	}
	return tx.Commit()
}

// BestiaryOf is a character's bestiary: by zone, then by kind (any case).
func (db *SQLiteDB) BestiaryOf(serverID uint32) ([]BestiaryEntry, error) {
	rows, err := db.Query(`SELECT kind, zone, first_seen, kills, last_kill FROM charbestiary WHERE serverid = ?
		ORDER BY zone, kind`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BestiaryEntry
	for rows.Next() {
		var e BestiaryEntry
		var first, last int64
		if err := rows.Scan(&e.Kind, &e.Zone, &first, &e.Kills, &last); err != nil {
			return nil, err
		}
		e.FirstSeen = time.Unix(first, 0).UTC()
		if last > 0 {
			e.LastKill = time.Unix(last, 0).UTC()
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
