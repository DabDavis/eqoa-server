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
// damage_pct its melee hits, con_pct how much of balancecon's effect its
// level takes (0 none: a low level's own hit points and floor already climb
// a third or more in three levels; 100 all of it), and pet_hp_pct and
// pet_damage_pct a summoned pet's of that level (its hit points, and all
// the damage it deals).
//
// balancecon, by an NPC's level minus its foe's (a player's, or a pet's
// own), the con: damage_pct the NPC's melee hits on that foe, taken_pct the
// damage the foe deals the NPC (melee, spells, damage over time, procs,
// shields; a pet's too). Between two rows the line; past the last, the end
// row's.
//
// balancelevel's art_cooldown_ms is by a PLAYER's level, the one column that
// is: the global cooldown of the combat arts (an ability with no cast time
// cast at a creature), the milliseconds from one's start before another may
// start (entity/balance.go ArtCooldown).
//
// A table with no rows changes nothing. source: gm, or seed for the rows
// this file gives a table it makes (the tuning of 2026-10-29,
// claude-notes/balance-20261029.md). A column added to a table made before it
// (balanceColumns) takes the seed's value on the rows still the seed's.
const balanceSchema = `
	CREATE TABLE IF NOT EXISTS balancelevel (
		level          INTEGER PRIMARY KEY,
		hp_pct         REAL NOT NULL DEFAULT 100,
		damage_pct     REAL NOT NULL DEFAULT 100,
		con_pct        REAL NOT NULL DEFAULT 100,
		pet_hp_pct     REAL NOT NULL DEFAULT 100,
		pet_damage_pct REAL NOT NULL DEFAULT 100,
		art_cooldown_ms REAL NOT NULL DEFAULT 0,
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
	ConPct                 float64
	Level                  int
	HPPct, DamagePct       float64
	PetHPPct, PetDamagePct float64
	// ArtCooldownMS is by a player's level (the file's doc)
	ArtCooldownMS float64
	Note          string
}

// balanceColumn is a column added to a balance table after the table was
// first made: its declaration, and the seed rows' value of it by key.
type balanceColumn struct {
	table, col, decl, key string
	seed                  func() map[int]float64
}

// balanceColumns are the columns added since 2026-10-29's first tables.
var balanceColumns = []balanceColumn{
	{"balancelevel", "art_cooldown_ms", "REAL NOT NULL DEFAULT 0", "level", func() map[int]float64 {
		m := map[int]float64{}
		for _, r := range BalanceSeedLevels {
			m[r.Level] = r.ArtCooldownMS
		}
		return m
	}},
}

// ensureBalanceColumns adds the columns a table made before them lacks, each
// with the seed's value on the rows still the seed's (source seed).
func (db *SQLiteDB) ensureBalanceColumns() error {
	for _, c := range balanceColumns {
		has, err := hasColumn(db.DB, c.table, c.col)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		if has {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.col + ` ` + c.decl); err != nil {
			return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
		}
		for k, v := range c.seed() {
			if _, err := db.Exec(`UPDATE `+c.table+` SET `+c.col+` = ? WHERE `+c.key+` = ? AND source = 'seed'`, v, k); err != nil {
				return fmt.Errorf("%s.%s: %w", c.table, c.col, err)
			}
		}
	}
	return nil
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
	if err := db.ensureBalanceColumns(); err != nil {
		return err
	}
	if fresh["balancelevel"] {
		for _, r := range BalanceSeedLevels {
			if _, err := db.Exec(`INSERT OR IGNORE INTO balancelevel (level, hp_pct, damage_pct, con_pct, pet_hp_pct, pet_damage_pct,
				art_cooldown_ms, note, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'seed')`, r.Level, r.HPPct, r.DamagePct, r.ConPct,
				r.PetHPPct, r.PetDamagePct, r.ArtCooldownMS, r.Note); err != nil {
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
	rows, err := db.Query(`SELECT level, hp_pct, damage_pct, con_pct, pet_hp_pct, pet_damage_pct, art_cooldown_ms, note
		FROM balancelevel ORDER BY level`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BalanceLevel
	for rows.Next() {
		var r BalanceLevel
		if err := rows.Scan(&r.Level, &r.HPPct, &r.DamagePct, &r.ConPct, &r.PetHPPct, &r.PetDamagePct, &r.ArtCooldownMS, &r.Note); err != nil {
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
// nothing: a percent below 1 or past 10000 (con_pct is checked as
// BalanceConPctProblem), a level below 1 or past 100, a diff past 20 either
// way.
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

// BalanceMSProblem is what is wrong with a time in milliseconds, "" for
// nothing: 0 (none) to a minute.
func BalanceMSProblem(col string, ms float64) string {
	if ms < 0 || ms > 60000 || ms != ms {
		return fmt.Sprintf("%s %g is not 0-60000", col, ms)
	}
	return ""
}

// BalanceConPctProblem is what is wrong with a con_pct, "" for nothing: it is
// 0 to 100.
func BalanceConPctProblem(p float64) string {
	if p < 0 || p > 100 || p != p {
		return fmt.Sprintf("con_pct %g is not 0-100", p)
	}
	return ""
}
