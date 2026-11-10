package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// heroes.go — the Testing Heroes' tables (entity/heroes.go, handlers/heroes.go;
// petition 1471, the GM Jibrish, 2026-11-10: "For all mobs that give the player
// XP, add a new faction "Testing Heroes" ... Add a vendor in freeport near the
// well and dabrish that allows you to buy items from him with that faction").
//
//   - heroes_stock: what the Testing Heroes Vendor sells, a row an item: its
//     record, what it costs of the buyer's standing with the faction, and what
//     he says of it before the buyer says yes (empty: the record's own
//     description). tools/seed_testing_heroes.py writes the petition's five
//     (source "testing-heroes"); a GM's row sells anything else. Read at every
//     talk.
//   - heroes_purchases: every purchase, written as it is made: when, who, the
//     item, its cost and the buyer's standing before and after. A log: nothing
//     reads it back.
//   - charbonustp: the training points a character was given past its levels'
//     (an item's: entity/potion.go, "Increase Training Points by N"), a row a
//     character. They are part of characters.totalTP, and this is how many of
//     it no level gave, so a level's top-up (entity.TopUpTraining) still gives
//     the level its own.

// HeroesFaction is the Testing Heroes' faction, as table factions names it.
const HeroesFaction = "Testing Heroes"

// HeroesStockSchema, HeroesPurchasesSchema and BonusTPSchema are the three
// tables (tools/seed_testing_heroes.py makes heroes_stock by the same words
// where a server has not yet: TestSeedTestingHeroesToolKeepsTheSchema).
const HeroesStockSchema = `CREATE TABLE IF NOT EXISTS heroes_stock (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	patternid INTEGER NOT NULL UNIQUE,
	cost      INTEGER NOT NULL,
	says      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT 'gm'
)`

const HeroesPurchasesSchema = `CREATE TABLE IF NOT EXISTS heroes_purchases (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	at              TEXT NOT NULL,
	serverid        INTEGER NOT NULL,
	charname        TEXT NOT NULL DEFAULT '',
	accountid       INTEGER NOT NULL DEFAULT 0,
	patternid       INTEGER NOT NULL,
	item            TEXT NOT NULL DEFAULT '',
	cost            INTEGER NOT NULL,
	standing_before INTEGER NOT NULL,
	standing_after  INTEGER NOT NULL,
	vendor          TEXT NOT NULL DEFAULT ''
)`

const BonusTPSchema = `CREATE TABLE IF NOT EXISTS charbonustp (
	serverid INTEGER PRIMARY KEY,
	points   INTEGER NOT NULL DEFAULT 0,
	updated  TEXT NOT NULL DEFAULT ''
)`

// ensureHeroes makes the Testing Heroes' tables (initSchema).
func (db *SQLiteDB) ensureHeroes() error {
	for _, q := range []struct{ name, schema string }{
		{"heroes_stock", HeroesStockSchema}, {"heroes_purchases", HeroesPurchasesSchema}, {"charbonustp", BonusTPSchema}} {
		if _, err := db.Exec(q.schema); err != nil {
			return fmt.Errorf("%s: %w", q.name, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_heroes_purchases_char ON heroes_purchases(serverid)`); err != nil {
		return fmt.Errorf("heroes_purchases: %w", err)
	}
	return nil
}

// HeroesStockRow is a row of heroes_stock.
type HeroesStockRow struct {
	ID        int
	PatternID int
	Cost      int
	Says      string
}

// HeroesStock is what the vendor sells: the rows with a cost above 0, the
// cheapest first (then as they were written).
func (db *SQLiteDB) HeroesStock() ([]HeroesStockRow, error) {
	rows, err := db.Query(`SELECT id, patternid, cost, says FROM heroes_stock WHERE cost > 0 ORDER BY cost, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeroesStockRow
	for rows.Next() {
		var r HeroesStockRow
		if err := rows.Scan(&r.ID, &r.PatternID, &r.Cost, &r.Says); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// HeroesPurchase is a purchase as heroes_purchases keeps it.
type HeroesPurchase struct {
	At                            time.Time
	ServerID                      uint32
	CharName                      string
	AccountID                     int
	PatternID                     int
	Item                          string
	Cost                          int
	StandingBefore, StandingAfter int
	Vendor                        string
}

// LogHeroesPurchase keeps a purchase.
func (db *SQLiteDB) LogHeroesPurchase(p HeroesPurchase) error {
	_, err := db.Exec(`INSERT INTO heroes_purchases (at, serverid, charname, accountid, patternid, item, cost,
		standing_before, standing_after, vendor) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.At.UTC().Format(time.RFC3339), p.ServerID, p.CharName, p.AccountID, p.PatternID, p.Item, p.Cost,
		p.StandingBefore, p.StandingAfter, p.Vendor)
	return err
}

// BonusTP is the training points a character was given past its levels'
// (charbonustp), 0 with no row.
func (db *SQLiteDB) BonusTP(serverID uint32) (int, error) {
	var n int
	err := db.QueryRow(`SELECT points FROM charbonustp WHERE serverid = ?`, serverID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return max(n, 0), err
}

// SetBonusTP keeps how many training points a character was given past its
// levels'.
func (db *SQLiteDB) SetBonusTP(serverID uint32, points int, at time.Time) error {
	_, err := db.Exec(`INSERT INTO charbonustp (serverid, points, updated) VALUES (?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET points = excluded.points, updated = excluded.updated`,
		serverID, max(points, 0), at.UTC().Format(time.RFC3339))
	return err
}

// BonusTP is the training points the character was given past its levels'
// (charbonustp).
func (s *SQLiteCharacterStore) BonusTP(serverID uint32) (int, error) { return s.db.BonusTP(serverID) }

// SetBonusTraining keeps a character's unspent and total training points and
// how many of the total no level gave, in one transaction: an item's gift
// (handlers/potion.go) is whole or not at all.
func (s *SQLiteCharacterStore) SetBonusTraining(serverID uint32, unused, total, bonus int, at time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE characters SET unusedTP = ?, totalTP = ? WHERE serverid = ?`, unused, total, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	if _, err := tx.Exec(`INSERT INTO charbonustp (serverid, points, updated) VALUES (?, ?, ?)
		ON CONFLICT (serverid) DO UPDATE SET points = excluded.points, updated = excluded.updated`,
		serverID, max(bonus, 0), at.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}
