package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// legacy.go — Legacy records (a GM's ruling, 2026-11-18, work board 1714:
// "all ascended should be no trade. Legacy existing items people have, set all
// new drops no trade if ascended"). Every Ascended record is NO TRADE since
// (ascendedColumns); the copies players held before the rule were moved, once
// a shard, to a Legacy record of each (tools/jib_lane3_20261122.py), which
// keeps them tradeable: an itempattern row of its own, listed in itemlegacy
// with the record it was split off, that record in every way (its figures,
// look, flags, use effects) but its name, "Legacy " and the record's, and its
// trade, 0. At every start, after the Ascended and the imported copies,
// SyncLegacyCopies makes each Legacy record its record again by that rule (fix
// the record, never its Legacy record). Wherever an item is known by its
// record or name (a weapon's proc, a worn hit effect, a hand-in, a loot step,
// a scroll, an item's spell, food, aura or cooldown), a Legacy record is its
// record (entity.LegacyRecord, LegacyBaseName, and the SQL here). No loot
// table, pool, shop or reward names a Legacy record: new drops are NO TRADE.

// LegacyPrefix is what a Legacy record's name is its record's after.
const LegacyPrefix = "Legacy "

// LegacySchema is table itemlegacy: each Legacy record (patternid), the record
// it was split off (record), when and by what.
const LegacySchema = `CREATE TABLE IF NOT EXISTS itemlegacy (
	patternid INTEGER PRIMARY KEY,
	record    INTEGER NOT NULL UNIQUE,
	made      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT ''
)`

// legacyRecordSQL is a patternid ? resolved to its Legacy record's record (the
// id itself when it is none); bind the id twice.
const legacyRecordSQL = `COALESCE((SELECT record FROM itemlegacy WHERE patternid = ?), ?)`

// ensureLegacy makes table itemlegacy (initSchema).
func (db *SQLiteDB) ensureLegacy() error {
	if _, err := db.Exec(LegacySchema); err != nil {
		return fmt.Errorf("itemlegacy: %w", err)
	}
	return nil
}

// LegacyBaseName is the name of the record a Legacy record's name was made
// from: name without the prefix, any case; name as it is without one.
func LegacyBaseName(name string) string {
	s := strings.TrimSpace(name)
	if len(s) >= len(LegacyPrefix) && strings.EqualFold(s[:len(LegacyPrefix)], LegacyPrefix) {
		return strings.TrimSpace(s[len(LegacyPrefix):])
	}
	return s
}

// LegacyMap is every Legacy record and its record (legacy -> record).
func LegacyMap(db *sql.DB) (map[int]int, error) {
	if _, err := db.Exec(LegacySchema); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT patternid, record FROM itemlegacy`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int]int{}
	for rows.Next() {
		var l, r int
		if err := rows.Scan(&l, &r); err != nil {
			return nil, err
		}
		m[l] = r
	}
	return m, rows.Err()
}

// legacyColumns are itempattern's columns but patternid, quoted (set), and
// what a Legacy record takes for each from its record (from).
func legacyColumns(cols []string) (set, from string) {
	var s, f []string
	for _, c := range cols {
		low := strings.ToLower(c)
		if low == "patternid" {
			continue
		}
		q := `"` + c + `"`
		s = append(s, q)
		switch low {
		case "itemname":
			f = append(f, `'`+LegacyPrefix+`' || trim(`+q+`)`)
		case "trade":
			f = append(f, "0")
		default:
			f = append(f, q)
		}
	}
	return strings.Join(s, ", "), strings.Join(f, ", ")
}

// LegacyCopyTx writes Legacy record id of record now (a new row, or the row
// there made its record again) with its use effects, and its itemlegacy row
// when it has none (source saying what made it).
func LegacyCopyTx(tx *sql.Tx, id, record int, source string, now time.Time) error {
	if _, err := tx.Exec(LegacySchema); err != nil {
		return err
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return err
	}
	set, from := legacyColumns(cols)
	if _, err := tx.Exec(`DELETE FROM itempattern WHERE patternid = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO itempattern ("patternid", %s) SELECT %d, %s FROM itempattern WHERE patternid = ?`,
		set, id, from), record); err != nil {
		return fmt.Errorf("Legacy record %d of record %d: %w", id, record, err)
	}
	if _, err := tx.Exec(`DELETE FROM itemeffect WHERE patternid = ?`, id); err != nil && !strings.Contains(err.Error(), "no such table") {
		return err
	}
	if err := copyItemEffects(tx, record, id, func(s string) string { return s }); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO itemlegacy (patternid, record, made, source) VALUES (?, ?, ?, ?)`,
		id, record, now.UTC().Format(time.RFC3339), source)
	return err
}

// SyncLegacyCopies makes every Legacy record its record again by the rule
// (the server's start, after the Ascended and the imported copies): only one
// that differs is written, its use effects copied again. It says how many
// records changed.
func SyncLegacyCopies(db *sql.DB, now time.Time) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(LegacySchema); err != nil {
		return 0, err
	}
	cols, err := tableColumns(tx, "itempattern")
	if err != nil {
		return 0, err
	}
	set, from := legacyColumns(cols)
	var pairs [][2]int
	rows, err := tx.Query(`SELECT l.patternid, l.record FROM itemlegacy l JOIN itempattern r ON r.patternid = l.record`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var l, r int
		if err := rows.Scan(&l, &r); err != nil {
			rows.Close()
			return 0, err
		}
		pairs = append(pairs, [2]int{l, r})
	}
	rows.Close()
	upd, err := tx.Prepare(fmt.Sprintf(`UPDATE itempattern SET (%s) = (SELECT %s FROM itempattern WHERE patternid = ?)
			WHERE patternid = ? AND (%s) IS NOT (SELECT %s FROM itempattern WHERE patternid = ?)`, set, from, set, from))
	if err != nil {
		return 0, err
	}
	defer upd.Close()
	changed := 0
	for _, p := range pairs {
		var there int
		if err := tx.QueryRow(`SELECT count(*) FROM itempattern WHERE patternid = ?`, p[0]).Scan(&there); err != nil {
			return 0, err
		}
		if there == 0 { // its row gone: made again
			if err := LegacyCopyTx(tx, p[0], p[1], "", now); err != nil {
				return 0, err
			}
			changed++
			continue
		}
		res, err := upd.Exec(p[1], p[0], p[1])
		if err != nil {
			return 0, fmt.Errorf("Legacy record %d of record %d: %w", p[0], p[1], err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed++
		}
		if _, err := tx.Exec(`DELETE FROM itemeffect WHERE patternid = ?`, p[0]); err != nil && !strings.Contains(err.Error(), "no such table") {
			return 0, err
		}
		if err := copyItemEffects(tx, p[1], p[0], func(s string) string { return s }); err != nil {
			return 0, err
		}
	}
	return changed, tx.Commit()
}
