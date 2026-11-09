package database

import "fmt"

// itemlists.go: where a charinventory row is (its location), and the rows a client
// that holds fewer than the character has are kept in (handlers/itemlists.go has the
// rule: CLIENT's pack and bank hold 40 records each, a larger client more).
//
// A row the character's client cannot hold is moved to the keeping of its list
// (LocPackKept, LocBankKept), its every other column as it was, and nothing in the
// game reads it there: GetCharacterItems and GetCharItem read the pack and the bank
// alone, so no request, sale, cast, quest or trade can reach it. It comes back to its
// list, at the end, at the first login whose client has room for it.

// The locations of a row.
const (
	LocPack     = 0 // the pack: what the character carries, worn items among it
	LocBank     = 1 // the bank
	LocPackKept = 2 // kept for the character: a pack row its client could not hold
	LocBankKept = 3 // kept for the character: a bank row its client could not hold
)

// heldLocations is the SQL condition on c.location for the rows a client holds.
const heldLocations = `c.location IN (0, 1)`

// ItemListRow is where a row is and its place: what handlers/itemlists.go's plan
// reads of every row a character has.
type ItemListRow struct {
	ItemID   int
	Location int // LocPack .. LocBankKept
	EquipLoc int // -1 not worn
	ListNum  int
}

// ItemListRows is every row of a character's, in each of its four places, in
// list order (location, listnumber, itemid: GetCharacterItems' order).
func (s *InventoryStore) ItemListRows(serverID uint32) ([]ItemListRow, error) {
	rows, err := s.db.Query(`SELECT itemid, location, equiploc, listnumber FROM charinventory
		WHERE serverid = ? AND location IN (0, 1, 2, 3) ORDER BY location, listnumber, itemid`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemListRow
	for rows.Next() {
		var r ItemListRow
		if err := rows.Scan(&r.ItemID, &r.Location, &r.EquipLoc, &r.ListNum); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ItemListMove is a row moved between a list and its keeping: from From to To,
// at ListNum.
type ItemListMove struct {
	ItemID   int
	From, To int
	ListNum  int
}

// MoveItemListRows makes the moves in one transaction, each only while its row
// is still the character's and where the move found it (From): moved[i] says
// whether moves[i] was made. A row's other columns (its stack, condition,
// charges, worn slot, quick slot) are left as they are.
func (s *InventoryStore) MoveItemListRows(serverID uint32, moves []ItemListMove) (moved []bool, err error) {
	moved = make([]bool, len(moves))
	if len(moves) == 0 {
		return moved, nil
	}
	for _, m := range moves {
		if m.From < LocPack || m.From > LocBankKept || m.To < LocPack || m.To > LocBankKept {
			return moved, fmt.Errorf("item %d: no location %d or %d", m.ItemID, m.From, m.To)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return moved, err
	}
	defer tx.Rollback()
	for i, m := range moves {
		res, err := tx.Exec(`UPDATE charinventory SET location = ?, listnumber = ? WHERE itemid = ? AND serverid = ? AND location = ?`,
			m.To, m.ListNum, m.ItemID, serverID, m.From)
		if err != nil {
			return make([]bool, len(moves)), err
		}
		if n, err := res.RowsAffected(); err == nil && n == 1 {
			moved[i] = true
		}
	}
	if err := tx.Commit(); err != nil {
		return make([]bool, len(moves)), err
	}
	return moved, nil
}

// KeptItemRows is how many of a character's rows are kept for it (LocPackKept,
// LocBankKept).
func (s *InventoryStore) KeptItemRows(serverID uint32) (pack, bank int, err error) {
	err = s.db.QueryRow(`SELECT COALESCE(SUM(location = 2), 0), COALESCE(SUM(location = 3), 0) FROM charinventory WHERE serverid = ?`,
		serverID).Scan(&pack, &bank)
	return pack, bank, err
}

// KeptPatterns is the items of a character's rows kept for it, by list: the
// pack's keeping (LocPackKept) and the bank's (LocBankKept).
func (s *InventoryStore) KeptPatterns(serverID uint32) (pack, bank map[int]bool, err error) {
	rows, err := s.db.Query(`SELECT patternid, location FROM charinventory WHERE serverid = ? AND location IN (2, 3)`, serverID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, loc int
		if err := rows.Scan(&id, &loc); err != nil {
			return nil, nil, err
		}
		m := &pack
		if loc == LocBankKept {
			m = &bank
		}
		if *m == nil {
			*m = map[int]bool{}
		}
		(*m)[id] = true
	}
	return pack, bank, rows.Err()
}
