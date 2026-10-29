package database

import (
	"fmt"
	"strings"
)

// balancelevel and balancecon: the game's balance (entity/balance.go), the
// GMs' to tune in the panel. Sandstorm's measured formulas stay as they are
// (offence.go, defence.go, stats.go); these scale what goes into a fight
// and what comes out of it.
//
// balancelevel, by an NPC's level (the levels between two rows take the
// straight line between them; below the first and past the last, the end
// row's): hp_pct its hit points (a percent of the server's 60 a level),
// damage_pct its melee hits, and pet_hp_pct and pet_damage_pct a summoned
// pet's of that level (its hit points, and all the damage it deals).
//
// balancecon, by an NPC's level minus its foe's (a player's, or a pet's
// own), the con: damage_pct the NPC's melee hits on that foe, taken_pct the
// damage the foe deals the NPC (melee, spells, damage over time, procs,
// shields; a pet's too). Between two rows the line; past the last, the end
// row's.
//
// A table with no rows changes nothing. source: gm, or seed for the rows
// this file gives a table it makes (the tuning of 2026-10-29,
// claude-notes/balance-20261029.md).
const balanceSchema = `
	CREATE TABLE IF NOT EXISTS balancelevel (
		level          INTEGER PRIMARY KEY,
		hp_pct         REAL NOT NULL DEFAULT 100,
		damage_pct     REAL NOT NULL DEFAULT 100,
		pet_hp_pct     REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100,
		note           TEXT NOT NULL DEFAULT '',
		source         TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS balancecon (
		diff       INTEGER PRIMARY KEY,
		damage_pct REAL NOT NULL DEFAULT 100,
		taken_pct  REAL NOT NULL DEFAULT 100,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);`

// BalanceLevel is one row of balancelevel.
type BalanceLevel struct {
	Level                  int
	HPPct, DamagePct       float64
	PetHPPct, PetDamagePct float64
	Note                   string
}

// BalanceCon is one row of balancecon.
type BalanceCon struct {
	Diff                int
	DamagePct, TakenPct float64
	Note                string
}

// ensureBalance makes tables balancelevel and balancecon (initSchema), each
// with its seed rows when it is made: a database that has the tables keeps
// what is in them, rows a GM took out included.
func (db *SQLiteDB) ensureBalance() error {
	fresh := map[string]bool{}
	for _, t := range []string{"balancelevel", "balancecon"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, t).Scan(&n); err != nil {
			return fmt.Errorf("%s: %w", t, err)
		}
		fresh[t] = n == 0
	}
	if _, err := db.Exec(balanceSchema); err != nil {
		return fmt.Errorf("balance: %w", err)
	}
	if fresh["balancelevel"] {
		for _, r := range BalanceSeedLevels {
			if _, err := db.Exec(`INSERT OR IGNORE INTO balancelevel (level, hp_pct, damage_pct, pet_hp_pct, pet_damage_pct, note, source)
				VALUES (?, ?, ?, ?, ?, ?, 'seed')`, r.Level, r.HPPct, r.DamagePct, r.PetHPPct, r.PetDamagePct, r.Note); err != nil {
				return fmt.Errorf("balancelevel: %w", err)
			}
		}
	}
	if fresh["balancecon"] {
		for _, r := range BalanceSeedCons {
			if _, err := db.Exec(`INSERT OR IGNORE INTO balancecon (diff, damage_pct, taken_pct, note, source)
				VALUES (?, ?, ?, ?, 'seed')`, r.Diff, r.DamagePct, r.TakenPct, r.Note); err != nil {
				return fmt.Errorf("balancecon: %w", err)
			}
		}
	}
	return nil
}

// BalanceLevels is every row of balancelevel, by level.
func (db *SQLiteDB) BalanceLevels() ([]BalanceLevel, error) {
	rows, err := db.Query(`SELECT level, hp_pct, damage_pct, pet_hp_pct, pet_damage_pct, note FROM balancelevel ORDER BY level`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BalanceLevel
	for rows.Next() {
		var r BalanceLevel
		if err := rows.Scan(&r.Level, &r.HPPct, &r.DamagePct, &r.PetHPPct, &r.PetDamagePct, &r.Note); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BalanceCons is every row of balancecon, by diff.
func (db *SQLiteDB) BalanceCons() ([]BalanceCon, error) {
	rows, err := db.Query(`SELECT diff, damage_pct, taken_pct, note FROM balancecon ORDER BY diff`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BalanceCon
	for rows.Next() {
		var r BalanceCon
		if err := rows.Scan(&r.Diff, &r.DamagePct, &r.TakenPct, &r.Note); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BalanceProblems is what is wrong with a row of either table, "" for
// nothing: a percent below 1 or past 10000, a level below 1 or past 100, a
// diff past 20 either way.
func BalanceProblems(table string, level int, pcts ...float64) string {
	var bad []string
	switch table {
	case "balancelevel":
		if level < 1 || level > 100 {
			bad = append(bad, fmt.Sprintf("level %d is not 1-100", level))
		}
	case "balancecon":
		if level < -20 || level > 20 {
			bad = append(bad, fmt.Sprintf("diff %d is not -20 to 20", level))
		}
	}
	for _, p := range pcts {
		if p < 1 || p > 10000 || p != p {
			bad = append(bad, fmt.Sprintf("a percent of %g is not 1-10000", p))
		}
	}
	return strings.Join(bad, "; ")
}
