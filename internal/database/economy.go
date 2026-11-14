package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// The economy's ledger (the owner, 2026-11-14: "lets add an economy tracker for
// the gm panel shows tunar accumulated from loot, selling, etc. and trend make
// it informative"): every movement of tunar is counted where it happens
// (internal/economy, in memory) and written here once a minute, an hour and a
// source a row, never a row an event; each character's own by the day; and
// once an hour what everyone holds. The GM panel's Economy page reads them
// (gmweb/ops_economy.go). Each shard its own, in its own database.
const economySchema = `
	CREATE TABLE IF NOT EXISTS tunar_ledger (
		hour   INTEGER NOT NULL,
		source TEXT NOT NULL,
		kind   TEXT NOT NULL,
		count  INTEGER NOT NULL DEFAULT 0,
		amount INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (hour, source)
	);
	CREATE TABLE IF NOT EXISTS tunar_ledger_chars (
		day      INTEGER NOT NULL,
		serverid INTEGER NOT NULL,
		source   TEXT NOT NULL,
		name     TEXT NOT NULL DEFAULT '',
		count    INTEGER NOT NULL DEFAULT 0,
		got      INTEGER NOT NULL DEFAULT 0,
		paid     INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (day, serverid, source)
	);
	CREATE TABLE IF NOT EXISTS tunar_wealth (
		hour       INTEGER PRIMARY KEY,
		characters INTEGER NOT NULL DEFAULT 0,
		holders    INTEGER NOT NULL DEFAULT 0,
		purse      INTEGER NOT NULL DEFAULT 0,
		bank       INTEGER NOT NULL DEFAULT 0,
		escrow     INTEGER NOT NULL DEFAULT 0,
		staff      INTEGER NOT NULL DEFAULT 0,
		median     INTEGER NOT NULL DEFAULT 0,
		p90        INTEGER NOT NULL DEFAULT 0,
		p99        INTEGER NOT NULL DEFAULT 0,
		top10      INTEGER NOT NULL DEFAULT 0,
		over_100k  INTEGER NOT NULL DEFAULT 0,
		over_1m    INTEGER NOT NULL DEFAULT 0,
		over_10m   INTEGER NOT NULL DEFAULT 0,
		top        TEXT NOT NULL DEFAULT '[]'
	);`

// ensureEconomy makes the ledger's tables (initSchema).
func (db *SQLiteDB) ensureEconomy() error {
	if _, err := db.Exec(economySchema); err != nil {
		return fmt.Errorf("economy tables: %w", err)
	}
	return nil
}

// TunarHourRow is one hour's count of one source.
type TunarHourRow struct {
	Hour   int64 // unix seconds, the hour's start (UTC)
	Source string
	Kind   string // faucet, sink, transfer
	Count  int64
	Amount int64
}

// TunarCharRow is one character's movements of one source in one day. Got is
// what it gained, Paid what it gave (a trade's side may do both in a day).
type TunarCharRow struct {
	Day       int64 // unix seconds, the day's start (UTC)
	ServerID  uint32
	Source    string
	Name      string
	Count     int64
	Got, Paid int64
}

// FlushTunarLedger adds rows to the ledger, in one transaction.
func (db *SQLiteDB) FlushTunarLedger(hours []TunarHourRow, chars []TunarCharRow) error {
	if len(hours) == 0 && len(chars) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range hours {
		if _, err := tx.Exec(`INSERT INTO tunar_ledger (hour, source, kind, count, amount) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (hour, source) DO UPDATE SET count = count + excluded.count, amount = amount + excluded.amount,
			kind = excluded.kind`, r.Hour, r.Source, r.Kind, r.Count, r.Amount); err != nil {
			return err
		}
	}
	for _, r := range chars {
		if _, err := tx.Exec(`INSERT INTO tunar_ledger_chars (day, serverid, source, name, count, got, paid) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (day, serverid, source) DO UPDATE SET count = count + excluded.count, got = got + excluded.got,
			paid = paid + excluded.paid, name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE name END`,
			r.Day, r.ServerID, r.Source, r.Name, r.Count, r.Got, r.Paid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TunarHolder is one character's tunar in a wealth snapshot.
type TunarHolder struct {
	ServerID uint32 `json:"serverid"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	Purse    int64  `json:"purse"`
	Bank     int64  `json:"bank"`
	Total    int64  `json:"total"`
	Staff    bool   `json:"staff,omitempty"`
}

// TunarWealth is what everyone held at an hour. The distribution (median,
// percentiles, the counts over a mark, the top ten's share) is the players'
// alone: a GM's character (acctlevel at or above the panel's GM level) is
// counted in Staff and the supply, never in the spread.
type TunarWealth struct {
	Hour       int64         `json:"hour"`
	Characters int           `json:"characters"` // players' characters
	Holders    int           `json:"holders"`    // of them, holding any tunar
	Purse      int64         `json:"purse"`      // players' purses
	Bank       int64         `json:"bank"`       // players' banks
	Escrow     int64         `json:"escrow"`     // held for the auction house's bids
	Staff      int64         `json:"staff"`      // GMs' characters, purses and banks
	Median     int64         `json:"median"`
	P90        int64         `json:"p90"`
	P99        int64         `json:"p99"`
	Top10      int64         `json:"top10"` // what the ten richest players hold
	Over100k   int           `json:"over100k"`
	Over1M     int           `json:"over1m"`
	Over10M    int           `json:"over10m"`
	Top        []TunarHolder `json:"top"` // the twenty richest, staff marked
}

// Supply is all the tunar there is: purses, banks, escrow, staff's too.
func (w TunarWealth) Supply() int64 { return w.Purse + w.Bank + w.Escrow + w.Staff }

// PlayerSupply is the players' alone, with the escrow.
func (w TunarWealth) PlayerSupply() int64 { return w.Purse + w.Bank + w.Escrow }

// MeasureTunarWealth reads everyone's tunar now (one pass over characters):
// staffLevel is the acctlevel from which an account is a GM's.
func MeasureTunarWealth(q Queryer, hour int64, staffLevel int) (TunarWealth, error) {
	w := TunarWealth{Hour: hour}
	rows, err := q.Query(`SELECT c.serverid, c.charName, COALESCE(c.level, 0), COALESCE(c.tunar, 0), COALESCE(c.bankTunar, 0),
		COALESCE(a.acctlevel, 0) FROM characters c LEFT JOIN accountinfo a ON a.accountid = c.accountid`)
	if err != nil {
		return w, err
	}
	var all []TunarHolder
	var totals []int64
	for rows.Next() {
		var h TunarHolder
		var lvl int
		if err := rows.Scan(&h.ServerID, &h.Name, &h.Level, &h.Purse, &h.Bank, &lvl); err != nil {
			rows.Close()
			return w, err
		}
		h.Total = h.Purse + h.Bank
		h.Staff = staffLevel > 0 && lvl >= staffLevel
		all = append(all, h)
		if h.Staff {
			w.Staff += h.Total
			continue
		}
		w.Characters++
		w.Purse += h.Purse
		w.Bank += h.Bank
		totals = append(totals, h.Total)
		if h.Total > 0 {
			w.Holders++
		}
		switch {
		case h.Total >= 10_000_000:
			w.Over10M++
			fallthrough
		case h.Total >= 1_000_000:
			w.Over1M++
			fallthrough
		case h.Total >= 100_000:
			w.Over100k++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return w, err
	}
	// escrow: the auction house's held bids (a database without one has none)
	var escrow sql.NullInt64
	if err := q.QueryRow(`SELECT SUM(escrow) FROM auction_bids WHERE escrow > 0`).Scan(&escrow); err == nil {
		w.Escrow = escrow.Int64
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i] < totals[j] })
	at := func(p float64) int64 {
		if len(totals) == 0 {
			return 0
		}
		i := int(p * float64(len(totals)-1))
		return totals[i]
	}
	w.Median, w.P90, w.P99 = at(0.5), at(0.9), at(0.99)
	for i := len(totals) - 1; i >= 0 && i >= len(totals)-10; i-- {
		w.Top10 += totals[i]
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Total > all[j].Total })
	if len(all) > 20 {
		all = all[:20]
	}
	w.Top = all
	return w, nil
}

// SaveTunarWealth keeps a snapshot (one an hour: the hour's again replaces it).
func (db *SQLiteDB) SaveTunarWealth(w TunarWealth) error {
	top, err := json.Marshal(w.Top)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO tunar_wealth (hour, characters, holders, purse, bank, escrow, staff, median,
		p90, p99, top10, over_100k, over_1m, over_10m, top) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.Hour, w.Characters, w.Holders, w.Purse, w.Bank, w.Escrow, w.Staff, w.Median, w.P90, w.P99, w.Top10,
		w.Over100k, w.Over1M, w.Over10M, string(top))
	return err
}

// HasTunarWealth reports a snapshot kept for the hour.
func (db *SQLiteDB) HasTunarWealth(hour int64) bool {
	var n int
	return db.QueryRow(`SELECT COUNT(*) FROM tunar_wealth WHERE hour = ?`, hour).Scan(&n) == nil && n > 0
}

// Queryer is what the economy's reads need: this database, or another
// shard's opened read-only.
type Queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// TunarHours is the ledger's rows from since (inclusive) to until (exclusive).
func TunarHours(q Queryer, since, until time.Time) ([]TunarHourRow, error) {
	rows, err := q.Query(`SELECT hour, source, kind, count, amount FROM tunar_ledger WHERE hour >= ? AND hour < ?
		ORDER BY hour, source`, since.Unix(), until.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TunarHourRow
	for rows.Next() {
		var r TunarHourRow
		if err := rows.Scan(&r.Hour, &r.Source, &r.Kind, &r.Count, &r.Amount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TunarChars is the characters' rows of the days from since to until.
func TunarChars(q Queryer, since, until time.Time) ([]TunarCharRow, error) {
	rows, err := q.Query(`SELECT day, serverid, source, name, count, got, paid FROM tunar_ledger_chars
		WHERE day >= ? AND day < ? ORDER BY day, serverid, source`, since.Unix(), until.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TunarCharRow
	for rows.Next() {
		var r TunarCharRow
		if err := rows.Scan(&r.Day, &r.ServerID, &r.Source, &r.Name, &r.Count, &r.Got, &r.Paid); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TunarWealthSince is the snapshots from since on, oldest first.
func TunarWealthSince(q Queryer, since time.Time) ([]TunarWealth, error) {
	rows, err := q.Query(`SELECT hour, characters, holders, purse, bank, escrow, staff, median, p90, p99, top10,
		over_100k, over_1m, over_10m, top FROM tunar_wealth WHERE hour >= ? ORDER BY hour`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TunarWealth
	for rows.Next() {
		var w TunarWealth
		var top string
		if err := rows.Scan(&w.Hour, &w.Characters, &w.Holders, &w.Purse, &w.Bank, &w.Escrow, &w.Staff, &w.Median,
			&w.P90, &w.P99, &w.Top10, &w.Over100k, &w.Over1M, &w.Over10M, &top); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(top), &w.Top)
		out = append(out, w)
	}
	return out, rows.Err()
}

// TunarLedgerFirst is the ledger's first hour (0: empty).
func TunarLedgerFirst(q Queryer) int64 {
	var h sql.NullInt64
	_ = q.QueryRow(`SELECT MIN(hour) FROM tunar_ledger`).Scan(&h)
	return h.Int64
}
