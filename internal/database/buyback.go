package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// buyback.go: the last things a character got rid of, which any merchant sells back (the user,
// 2026-11-07, after a player destroyed an item by accident: "lets give it back and add a buy back to
// merchants for last 5 items"). One row an action: a stack sold or destroyed at once is one row,
// with what it held and what it costs to have back. Only a merchant's sale and the item menu's
// Destroy make one (handlers/buyback.go); a corpse, the bank, a trade and the auction do not.

// BuyBackKept is how many a character keeps: a sixth pushes out the oldest.
const BuyBackKept = 5

// How an entry went.
const (
	BuyBackSold      = "sold"
	BuyBackDestroyed = "destroyed"
)

// ErrBuyBackGone is an entry that is not there to take: bought back already, pushed out, or taken
// away with its record.
var ErrBuyBackGone = errors.New("no such buy-back entry")

const charBuyBackSchema = `
	CREATE TABLE IF NOT EXISTS charbuyback (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid  INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		stack     INTEGER NOT NULL,
		remainhp  INTEGER NOT NULL,
		charges   INTEGER NOT NULL,
		price     INTEGER NOT NULL,
		how       TEXT NOT NULL,
		made      TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_charbuyback_serverid ON charbuyback(serverid, id);`

func (db *SQLiteDB) ensureCharBuyBack() error {
	if _, err := db.Exec(charBuyBackSchema); err != nil {
		return fmt.Errorf("charbuyback: %w", err)
	}
	return nil
}

// buyBackDead takes a character's entries that cannot come back: their record taken away, or No
// Rent now, which vanishes at a logout and so must not return this way (norent.go).
const buyBackDead = `DELETE FROM charbuyback WHERE serverid = ? AND (
	NOT EXISTS (SELECT 1 FROM itempattern p WHERE p.patternid = charbuyback.patternid AND p.rent <> 0)
	OR EXISTS (SELECT 1 FROM norentvanish v WHERE v.patternid = charbuyback.patternid))`

// BuyBackEntry is one thing a character got rid of.
type BuyBackEntry struct {
	ID int64
	// Item is the record as it is now in the character's own view (heirlooms.go), with what went:
	// StackLeft, RemainHP and Charges. It has no row, so no ItemID, and is not worn.
	Item CharInventoryEntry
	// Price is the tunar for the whole entry: what the merchant paid, or what one would have.
	Price int
	How   string
}

// AddBuyBack keeps n of row for the character at price, the newest of its entries, the oldest
// pushed out past BuyBackKept. A No Rent record (rent 0, what a spell conjures and what vanishes at a
// logout, or a row of norentvanish) is not kept: false and no error.
func (s *InventoryStore) AddBuyBack(serverID uint32, row *CharInventoryEntry, n, price int, how string) (int64, bool, error) {
	if row.Pattern.Rent == 0 {
		return 0, false, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var vanishes int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM norentvanish WHERE patternid = ?`, row.Pattern.PatternID).Scan(&vanishes); err != nil {
		return 0, false, err
	}
	if vanishes > 0 {
		return 0, false, nil
	}
	if _, err := tx.Exec(buyBackDead, serverID); err != nil {
		return 0, false, err
	}
	res, err := tx.Exec(`INSERT INTO charbuyback (serverid, patternid, stack, remainhp, charges, price, how, made)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		serverID, row.Pattern.PatternID, n, row.RemainHP, row.Charges, price, how, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.Exec(`DELETE FROM charbuyback WHERE serverid = ? AND id NOT IN
		(SELECT id FROM charbuyback WHERE serverid = ? ORDER BY id DESC LIMIT ?)`, serverID, serverID, BuyBackKept); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// BuyBacks is the character's entries, newest first. One that cannot come back is dropped quietly,
// here and from the table.
func (s *InventoryStore) BuyBacks(serverID uint32) ([]BuyBackEntry, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM charbuyback WHERE serverid = ?`, serverID).Scan(&total); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT b.id, b.stack, b.remainhp, b.charges, b.price, b.how, `+itemPatternColumns+`
		FROM charbuyback b JOIN itempattern p ON p.patternid = b.patternid
		WHERE b.serverid = ? AND p.rent <> 0
			AND NOT EXISTS (SELECT 1 FROM norentvanish v WHERE v.patternid = b.patternid)
		ORDER BY b.id DESC`, serverID)
	if err != nil {
		return nil, err
	}
	var out []BuyBackEntry
	for rows.Next() {
		var e BuyBackEntry
		e.Item.EquipLoc = -1
		dest := append([]any{&e.ID, &e.Item.StackLeft, &e.Item.RemainHP, &e.Item.Charges, &e.Price, &e.How}, patternDest(&e.Item.Pattern)...)
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	rows.Close() // one connection: nothing else while the rows are open
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) < total {
		if _, err := s.db.Exec(buyBackDead, serverID); err != nil {
			return nil, err
		}
	}
	for i := range out {
		s.db.ViewPattern(serverID, &out[i].Item.Pattern)
	}
	return out, nil
}

// TakeBuyBack gives the entry back in one transaction, so it can neither be had twice nor be lost:
// the entry leaves the table (ErrBuyBackGone and nothing done when it has already), its item goes
// into the pack as it went, its condition and charges exactly, and the purse becomes purse. onto is
// the row of the stack it joins, which then holds ontoStack, or 0 for a row of its own at the end of the
// pack. It returns the item's row.
func (s *InventoryStore) TakeBuyBack(serverID uint32, id int64, onto, ontoStack, purse int) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var pattern, stack, hp, charges int
	err = tx.QueryRow(`SELECT patternid, stack, remainhp, charges FROM charbuyback WHERE id = ? AND serverid = ?`, id, serverID).
		Scan(&pattern, &stack, &hp, &charges)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrBuyBackGone
	}
	if err != nil {
		return 0, err
	}
	one := func(res sql.Result, err error) error {
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n != 1 {
			return fmt.Errorf("%d rows changed, want 1", n)
		}
		return nil
	}
	if err := one(tx.Exec(`DELETE FROM charbuyback WHERE id = ? AND serverid = ?`, id, serverID)); err != nil {
		return 0, err
	}
	itemID := onto
	if onto > 0 {
		if err := one(tx.Exec(`UPDATE charinventory SET stackleft = ? WHERE itemid = ? AND serverid = ?`, ontoStack, onto, serverID)); err != nil {
			return 0, err
		}
	} else {
		res, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, ?, ?, ?, ?, -1, 0,
				(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = 0))`,
			serverID, stack, hp, charges, pattern, serverID)
		if err != nil {
			return 0, err
		}
		last, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		itemID = int(last)
	}
	if err := one(tx.Exec(`UPDATE characters SET tunar = ? WHERE serverid = ?`, purse, serverID)); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return itemID, nil
}
