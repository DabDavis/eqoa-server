package database

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
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
	// More is more rows given up with it, each as ItemID is (a pair's other,
	// Heirloom petition 2236: the rogue's epic daggers are given two at once)
	More []int
	// Count is how many copies of the record are given (0: one)
	Count int
	From  int // the record the row must still be of
	// To is the record given for it; with ImportedOf set instead, ImportedOf's
	// imported copy, made in the same transaction when it has none
	// (ImportedCopyTx: the transfer's damper follows the item)
	To, ImportedOf int
	Source         string // what made a copy made here
	// With is a set's other rows given up with ItemID, each as ItemID is but
	// of its own record, and Also the set's other records given with To
	// (Heirloom petition 2550: the bard's level 60 pick gives the Ascended
	// Blades of Lyrical Harmony and Refrain as one option, two records)
	With []ExchangeRow
	Also []ExchangeGive
}

// ExchangeRow is a row given up: its id and the record it must still be of.
type ExchangeRow struct{ ItemID, From int }

// ExchangeGive is a record given: To, or with ImportedOf set instead,
// ImportedOf's imported copy (made when it has none), as ItemExchange's.
type ExchangeGive struct{ To, ImportedOf int }

// ExchangeGiven is a record given and its new row, its condition and charges.
type ExchangeGiven struct{ ItemID, To, RemainHP, Charges int }

// ErrExchangeGone is an exchange whose row is no longer in the pack as it was
// (worn since, banked, sold, traded).
var ErrExchangeGone = errors.New("the item is no longer in the pack as it was")

// ItemExchanged is an exchange done: the new row and its record, its
// condition and charges.
type ItemExchanged struct {
	ItemID, To, RemainHP, Charges int
	More                          []int           // the other copies' rows (ItemExchange.Count)
	Also                          []ExchangeGiven // ItemExchange.Also's, in order
}

// ExchangeCharItem gives the character the record x names for the row x
// gives up, in one transaction: the row deleted (off its quick slot), a new
// row of the record at the end of the pack, unworn, its condition the old
// one's share of its durability (full when the old record had none) and its
// charges the old row's, at most the new record's full (its full when either
// has none). x's More and With rows go with it, and each record of x.Also is
// given as To is, by the same rules from the same row. Nothing changes when a
// row is not there as x says (ErrExchangeGone).
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
	gone := []int{x.ItemID}
	for _, id := range x.More {
		gone = append(gone, id)
		if err := exchangeRowThere(tx, serverID, ExchangeRow{ItemID: id, From: x.From}); err != nil {
			return out, err
		}
	}
	for _, r := range x.With {
		gone = append(gone, r.ItemID)
		if err := exchangeRowThere(tx, serverID, r); err != nil {
			return out, err
		}
	}
	for i, id := range gone {
		if slices.Contains(gone[:i], id) {
			return out, ErrExchangeGone
		}
	}
	first := ExchangeGive{To: x.To, ImportedOf: x.ImportedOf}
	var given []ExchangeGiven
	for i, g := range append([]ExchangeGive{first}, x.Also...) {
		r, err := exchangeGiven(tx, g, x.Source, remain, charges, fromMax, now)
		if err != nil {
			return out, err
		}
		if i == 0 {
			out.To, out.RemainHP, out.Charges = r.To, r.RemainHP, r.Charges
		}
		given = append(given, r)
	}
	for _, id := range gone {
		if _, err := tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, id, serverID); err != nil {
			return out, err
		}
		if _, err := tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, id); err != nil &&
			!isNoSuchTable(err) {
			return out, err
		}
	}
	insert := func(g ExchangeGiven) (int, error) {
		res, err := tx.Exec(`
			INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
			VALUES (?, 1, ?, ?, ?, -1, ?,
				(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))
		`, serverID, g.RemainHP, g.Charges, g.To, LocPack, serverID, LocPack)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		return int(id), err
	}
	for i := 0; i < max(1, x.Count); i++ {
		id, err := insert(given[0])
		if err != nil {
			return out, err
		}
		if i == 0 {
			out.ItemID = id
		} else {
			out.More = append(out.More, id)
		}
	}
	for _, g := range given[1:] {
		if g.ItemID, err = insert(g); err != nil {
			return out, err
		}
		out.Also = append(out.Also, g)
	}
	return out, tx.Commit()
}

// exchangeRowThere is nil when the row is in the character's pack, unworn,
// one of r.From; else ErrExchangeGone.
func exchangeRowThere(tx *sql.Tx, serverID uint32, r ExchangeRow) error {
	var n int
	err := tx.QueryRow(`SELECT stackleft FROM charinventory
		WHERE itemid = ? AND serverid = ? AND patternid = ? AND location = ? AND equiploc < 0`,
		r.ItemID, serverID, r.From, LocPack).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && n != 1) {
		return ErrExchangeGone
	}
	return err
}

// exchangeGiven is a record an exchange gives, its new row's condition and
// charges (ItemExchange's rules, from the row given up: remain of fromMax,
// its charges), the imported copy made when g asks one; its ItemID not yet set.
func exchangeGiven(tx *sql.Tx, g ExchangeGive, source string, remain, charges, fromMax int, now time.Time) (ExchangeGiven, error) {
	out := ExchangeGiven{To: g.To}
	if g.ImportedOf > 0 {
		var err error
		if out.To, _, err = ImportedCopyTx(tx, g.ImportedOf, source, now); err != nil {
			return out, fmt.Errorf("record %d's imported copy: %w", g.ImportedOf, err)
		}
	}
	if out.To <= 0 {
		return out, errors.New("no record to give")
	}
	var toMax, toStack int
	if err := tx.QueryRow(`SELECT COALESCE(maxhp, 0), COALESCE(maxstack, 0) FROM itempattern WHERE patternid = ?`, out.To).
		Scan(&toMax, &toStack); err != nil {
		return out, fmt.Errorf("record %d: %w", out.To, err)
	}
	if toStack >= 2 {
		return out, fmt.Errorf("record %d stacks", out.To)
	}
	out.RemainHP = toMax
	if fromMax > 0 && toMax > 0 {
		out.RemainHP = min(toMax, max(0, remain)*toMax/fromMax)
	}
	full := 0
	if err := tx.QueryRow(`SELECT charges FROM itemeffect WHERE patternid = ?`, out.To).Scan(&full); err != nil &&
		!errors.Is(err, sql.ErrNoRows) && !isNoSuchTable(err) {
		return out, err
	}
	out.Charges = full
	if full > 0 && charges > 0 {
		out.Charges = min(charges, full)
	}
	return out, nil
}

// isNoSuchTable is an error of a table a database does not have (one older
// than it, or a test's).
func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}
