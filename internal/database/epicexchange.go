package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// epicexchange.go — what Dabrish's epic exchange (handlers/dabrishepic.go)
// reads and writes: the records' versions (an Ascended record's base, an
// imported copy's base), whether a character owns any of some records, and
// the exchange itself, one row for another in one transaction.

// ItemVersions is the copies' bases: itemascended's and itemimported's (copy ->
// base), read now.
func (s *InventoryStore) ItemVersions() (ascended, imported map[int]int, err error) {
	if ascended, err = AscendedMap(s.db.DB); err != nil {
		return nil, nil, err
	}
	if imported, err = ImportedMap(s.db.DB); err != nil {
		return nil, nil, err
	}
	return ascended, imported, nil
}

// OwnsAnyOf is whether the character has a row of any of the records
// anywhere: carried, worn, banked, or kept beside a smaller client's lists
// (itemlists.go).
func (s *InventoryStore) OwnsAnyOf(serverID uint32, patterns []int) (bool, error) {
	for _, p := range patterns {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, serverID, p).
			Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ItemExchange is one row of a character's pack given up for a record.
type ItemExchange struct {
	ItemID int // the row given up: in the pack, not worn, one of a record that does not stack
	From   int // the record the row must still be of
	// To is the record given for it; with ImportedOf set instead, ImportedOf's
	// imported copy, made in the same transaction when it has none
	// (ImportedCopyTx: the transfer's damper follows the item)
	To, ImportedOf int
	Source         string // what made a copy made here
}

// ErrExchangeGone is an exchange whose row is no longer in the pack as it was
// (worn since, banked, sold, traded).
var ErrExchangeGone = errors.New("the item is no longer in the pack as it was")

// ItemExchanged is an exchange done: the new row and its record, its
// condition and charges.
type ItemExchanged struct {
	ItemID, To, RemainHP, Charges int
}

// ExchangeCharItem gives the character the record x names for the row x
// gives up, in one transaction: the row deleted (off its quick slot), a new
// row of the record at the end of the pack, unworn, its condition the old
// one's share of its durability (full when the old record had none) and its
// charges the old row's, at most the new record's full (its full when either
// has none). Nothing changes when the row is not there as x says
// (ErrExchangeGone).
func (s *InventoryStore) ExchangeCharItem(serverID uint32, x ItemExchange, now time.Time) (ItemExchanged, error) {
	var out ItemExchanged
	tx, err := s.db.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var stack, remain, charges, fromMax int
	err = tx.QueryRow(`SELECT c.stackleft, c.remainHP, c.remaincharge, COALESCE(p.maxhp, 0)
		FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
		WHERE c.itemid = ? AND c.serverid = ? AND c.patternid = ? AND c.location = ? AND c.equiploc < 0`,
		x.ItemID, serverID, x.From, LocPack).Scan(&stack, &remain, &charges, &fromMax)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && stack != 1) {
		return out, ErrExchangeGone
	}
	if err != nil {
		return out, err
	}
	to := x.To
	if x.ImportedOf > 0 {
		if to, _, err = ImportedCopyTx(tx, x.ImportedOf, x.Source, now); err != nil {
			return out, fmt.Errorf("record %d's imported copy: %w", x.ImportedOf, err)
		}
	}
	if to <= 0 {
		return out, errors.New("no record to give")
	}
	var toMax, toStack int
	if err := tx.QueryRow(`SELECT COALESCE(maxhp, 0), COALESCE(maxstack, 0) FROM itempattern WHERE patternid = ?`, to).
		Scan(&toMax, &toStack); err != nil {
		return out, fmt.Errorf("record %d: %w", to, err)
	}
	if toStack >= 2 {
		return out, fmt.Errorf("record %d stacks", to)
	}
	out.To = to
	out.RemainHP = toMax
	if fromMax > 0 && toMax > 0 {
		out.RemainHP = min(toMax, max(0, remain)*toMax/fromMax)
	}
	full := 0
	if err := tx.QueryRow(`SELECT charges FROM itemeffect WHERE patternid = ?`, to).Scan(&full); err != nil &&
		!errors.Is(err, sql.ErrNoRows) && !isNoSuchTable(err) {
		return out, err
	}
	out.Charges = full
	if full > 0 && charges > 0 {
		out.Charges = min(charges, full)
	}
	if _, err := tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, x.ItemID, serverID); err != nil {
		return out, err
	}
	if _, err := tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, x.ItemID); err != nil &&
		!isNoSuchTable(err) {
		return out, err
	}
	res, err := tx.Exec(`
		INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, 1, ?, ?, ?, -1, ?,
			(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))
	`, serverID, out.RemainHP, out.Charges, to, LocPack, serverID, LocPack)
	if err != nil {
		return out, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return out, err
	}
	out.ItemID = int(id)
	return out, tx.Commit()
}

// isNoSuchTable is an error of a table a database does not have (one older
// than it, or a test's).
func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}
