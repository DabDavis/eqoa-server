package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// charmercitem: what a character's mercs hold (handlers/mercinv.go; the
// owner, 2026-11-15: "lets add inventory to mercs same as players, can equip
// or hold items"), a row a record: the merc's slot, the item and its stack,
// condition and charges as they were in its owner's pack, and where it is
// worn (an equip slot as charinventory.equiploc, -1 carried). Slot 0 is no
// merc's: the keeping, what came back from a merc whose hire ended
// (DeleteMerc moves its rows there in the same transaction) and waits for
// room in its owner's pack. An item leaves this table only in the
// transaction that puts it into charinventory (MercItemToPack), or used up by
// its merc (UseMercItem), so none is lost between the two.
//
// charmercstock: the potions a character's alchemist mercs have made (the
// owner: "the alc mercs should constantly make pots and they are saved for
// future mercs also"), by the owner and the item, kept whatever becomes of
// the mercs.
const charMercItemSchema = `
	CREATE TABLE IF NOT EXISTS charmercitem (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		serverid  INTEGER NOT NULL,
		slot      INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		stack     INTEGER NOT NULL DEFAULT 1,
		remainhp  INTEGER NOT NULL DEFAULT 0,
		charges   INTEGER NOT NULL DEFAULT 0,
		equiploc  INTEGER NOT NULL DEFAULT -1,
		added     TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_charmercitem_owner ON charmercitem(serverid, slot);
	CREATE TABLE IF NOT EXISTS charmercstock (
		serverid  INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		count     INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (serverid, patternid)
	);`

// MercKeeping is the slot of charmercitem's rows that wait for their owner's
// pack (the hire that held them ended).
const MercKeeping = 0

// MercItemsMax is the records a merc holds, worn and carried: a pack's 40.
const MercItemsMax = 40

// ensureMercItems makes tables charmercitem and charmercstock (ensureMercs).
func (db *SQLiteDB) ensureMercItems() error {
	if _, err := db.Exec(charMercItemSchema); err != nil {
		return fmt.Errorf("charmercitem: %w", err)
	}
	return nil
}

// MercItem is one record a merc holds, its item's record joined.
type MercItem struct {
	ID        int
	ServerID  uint32
	Slot      int
	Stack     int
	RemainHP  int
	Charges   int
	EquipLoc  int
	PatternID int
	Pattern   ItemPatternRecord
}

// MercItems is what the character's merc of the slot holds (MercKeeping: the
// keeping), worn first in equip-slot order, then carried in the order given.
// A row whose item record is gone is kept and left out.
func (db *SQLiteDB) MercItems(serverID uint32, slot int) ([]MercItem, error) {
	rows, err := db.Query(`SELECT id, stack, remainhp, charges, equiploc, patternid FROM charmercitem
		WHERE serverid = ? AND slot = ? ORDER BY equiploc < 0, equiploc, id`, serverID, slot)
	if err != nil {
		return nil, err
	}
	var out []MercItem
	for rows.Next() {
		it := MercItem{ServerID: serverID, Slot: slot}
		if err := rows.Scan(&it.ID, &it.Stack, &it.RemainHP, &it.Charges, &it.EquipLoc, &it.PatternID); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	store := NewItemPatternStore(db)
	keep := out[:0]
	for _, it := range out {
		rec, err := store.GetItemPattern(it.PatternID)
		if err != nil || rec == nil {
			continue
		}
		it.Pattern = *rec
		keep = append(keep, it)
	}
	return keep, nil
}

// ErrMercItemWorn refuses to give a merc an item its owner wears.
var ErrMercItemWorn = errors.New("worn")

// ErrMercFull refuses an item to a merc holding MercItemsMax records.
var ErrMercFull = errors.New("the mercenary holds all it can")

// GiveMercItem moves n of the character's pack row itemID to its merc of the
// slot, in one transaction: off the pack's row (the whole row, or its stack
// less n) and onto the merc, onto a carried stack of the same item that has
// room (maxStack), else as a record of its own. It returns the merc's record's
// id and whether the pack's row is gone.
func (db *SQLiteDB) GiveMercItem(serverID uint32, slot, itemID, n, maxStack int) (mercID int, rowGone bool, err error) {
	if slot <= MercKeeping || n < 1 {
		return 0, false, errors.New("no merc slot or nothing to give")
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	var pattern, stack, hp, charges, equip int
	err = tx.QueryRow(`SELECT patternid, stackleft, remainHP, remaincharge, equiploc FROM charinventory
		WHERE itemid = ? AND serverid = ? AND location = ?`, itemID, serverID, LocPack).Scan(&pattern, &stack, &hp, &charges, &equip)
	if err != nil {
		return 0, false, fmt.Errorf("pack row %d: %w", itemID, err)
	}
	if equip >= 0 {
		return 0, false, ErrMercItemWorn
	}
	stack = max(stack, 1)
	if n > stack {
		return 0, false, fmt.Errorf("pack row %d holds %d, not %d", itemID, stack, n)
	}
	if n == stack {
		_, err = tx.Exec(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, itemID, serverID)
		if err == nil {
			_, err = tx.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, itemID)
		}
	} else {
		_, err = tx.Exec(`UPDATE charinventory SET stackleft = ? WHERE itemid = ? AND serverid = ?`, stack-n, itemID, serverID)
	}
	if err != nil {
		return 0, false, err
	}
	if maxStack > 1 {
		var onto, have int
		e := tx.QueryRow(`SELECT id, stack FROM charmercitem WHERE serverid = ? AND slot = ? AND patternid = ? AND equiploc < 0
			AND stack + ? <= ? ORDER BY id LIMIT 1`, serverID, slot, pattern, n, maxStack).Scan(&onto, &have)
		if e == nil {
			if _, err := tx.Exec(`UPDATE charmercitem SET stack = ? WHERE id = ?`, have+n, onto); err != nil {
				return 0, false, err
			}
			return onto, n == stack, tx.Commit()
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return 0, false, e
		}
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM charmercitem WHERE serverid = ? AND slot = ?`, serverID, slot).Scan(&count); err != nil {
		return 0, false, err
	}
	if count >= MercItemsMax {
		return 0, false, ErrMercFull
	}
	res, err := tx.Exec(`INSERT INTO charmercitem (serverid, slot, patternid, stack, remainhp, charges, equiploc, added)
		VALUES (?, ?, ?, ?, ?, ?, -1, ?)`, serverID, slot, pattern, n, hp, charges, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, false, err
	}
	id, _ := res.LastInsertId()
	return int(id), n == stack, tx.Commit()
}

// MercEquipChange is one merc record's equip slot changing (-1 carried).
type MercEquipChange struct {
	ID       int
	EquipLoc int
}

// SetMercEquip writes the merc's records' equip slots, all or none.
func (db *SQLiteDB) SetMercEquip(serverID uint32, slot int, changes []MercEquipChange) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range changes {
		res, err := tx.Exec(`UPDATE charmercitem SET equiploc = ? WHERE id = ? AND serverid = ? AND slot = ?`, c.EquipLoc, c.ID, serverID, slot)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("merc record %d is not slot %d's", c.ID, slot)
		}
	}
	return tx.Commit()
}

// UseMercItem takes n of the merc's record id (a potion thrown or drunk, a
// meal eaten): its stack less n, the record gone with the last.
func (db *SQLiteDB) UseMercItem(serverID uint32, id, n int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stack int
	if err := tx.QueryRow(`SELECT stack FROM charmercitem WHERE id = ? AND serverid = ?`, id, serverID).Scan(&stack); err != nil {
		return err
	}
	if stack <= n {
		_, err = tx.Exec(`DELETE FROM charmercitem WHERE id = ?`, id)
	} else {
		_, err = tx.Exec(`UPDATE charmercitem SET stack = ? WHERE id = ?`, stack-n, id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// MercItemToPack moves n of the character's merc record id (any slot, the
// keeping too) into its pack, in one transaction: onto the pack's row onto
// (a stack of the same item: its stack plus n; 0 none), else as a new row at
// the end of the pack (unequipped); the merc's record less n, gone with the
// last. It returns the pack row's id.
func (db *SQLiteDB) MercItemToPack(serverID uint32, id, n, onto int) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var pattern, stack, hp, charges int
	if err := tx.QueryRow(`SELECT patternid, stack, remainhp, charges FROM charmercitem WHERE id = ? AND serverid = ?`, id, serverID).
		Scan(&pattern, &stack, &hp, &charges); err != nil {
		return 0, err
	}
	stack = max(stack, 1)
	if n < 1 || n > stack {
		return 0, fmt.Errorf("merc record %d holds %d, not %d", id, stack, n)
	}
	itemID, err := packIn(tx, serverID, pattern, n, hp, charges, onto)
	if err != nil {
		return 0, err
	}
	if n == stack {
		_, err = tx.Exec(`DELETE FROM charmercitem WHERE id = ?`, id)
	} else {
		_, err = tx.Exec(`UPDATE charmercitem SET stack = ? WHERE id = ?`, stack-n, id)
	}
	if err != nil {
		return 0, err
	}
	return itemID, tx.Commit()
}

// packIn puts n of the item into the character's pack within tx: onto the
// row onto (0: none), else a new row at the pack's end. It returns the row's
// id.
func packIn(tx *sql.Tx, serverID uint32, pattern, n, hp, charges, onto int) (int, error) {
	if onto > 0 {
		res, err := tx.Exec(`UPDATE charinventory SET stackleft = stackleft + ? WHERE itemid = ? AND serverid = ? AND location = ? AND patternid = ?`,
			n, onto, serverID, LocPack, pattern)
		if err != nil {
			return 0, err
		}
		if k, _ := res.RowsAffected(); k != 1 {
			return 0, fmt.Errorf("pack row %d is not one of item %d", onto, pattern)
		}
		return onto, nil
	}
	res, err := tx.Exec(`INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, ?, ?, ?, ?, -1, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))`,
		serverID, n, hp, charges, pattern, LocPack, serverID, LocPack)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// MercStock is the character's alchemist mercs' potions made, by item.
func (db *SQLiteDB) MercStock(serverID uint32) (map[int]int, error) {
	rows, err := db.Query(`SELECT patternid, count FROM charmercstock WHERE serverid = ? AND count > 0`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// AddMercStock adds n of the item to the character's stock, the whole stock
// held to most potions (0: no cap); it reports how many went in.
func (db *SQLiteDB) AddMercStock(serverID uint32, patternID, n, most int) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if most > 0 {
		var have int
		if err := tx.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM charmercstock WHERE serverid = ?`, serverID).Scan(&have); err != nil {
			return 0, err
		}
		n = min(n, most-have)
	}
	if n <= 0 {
		return 0, nil
	}
	if _, err := tx.Exec(`INSERT INTO charmercstock (serverid, patternid, count) VALUES (?, ?, ?)
		ON CONFLICT(serverid, patternid) DO UPDATE SET count = count + excluded.count`, serverID, patternID, n); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// UseMercStock takes up to n of the item from the character's stock (a merc
// threw or drank one); it reports how many it took.
func (db *SQLiteDB) UseMercStock(serverID uint32, patternID, n int) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var have int
	if err := tx.QueryRow(`SELECT count FROM charmercstock WHERE serverid = ? AND patternid = ?`, serverID, patternID).Scan(&have); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	n = min(n, have)
	if n <= 0 {
		return 0, nil
	}
	if _, err := tx.Exec(`UPDATE charmercstock SET count = count - ? WHERE serverid = ? AND patternid = ?`, n, serverID, patternID); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// MercStockToPack moves n of the item from the character's stock into its
// pack in one transaction (onto as MercItemToPack's), the pack's row id
// returned; refused when the stock holds fewer.
func (db *SQLiteDB) MercStockToPack(serverID uint32, patternID, n, hp, onto int) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var have int
	if err := tx.QueryRow(`SELECT count FROM charmercstock WHERE serverid = ? AND patternid = ?`, serverID, patternID).Scan(&have); err != nil {
		return 0, err
	}
	if n < 1 || n > have {
		return 0, fmt.Errorf("the stock holds %d of item %d, not %d", have, patternID, n)
	}
	itemID, err := packIn(tx, serverID, patternID, n, hp, 0, onto)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE charmercstock SET count = count - ? WHERE serverid = ? AND patternid = ?`, n, serverID, patternID); err != nil {
		return 0, err
	}
	return itemID, tx.Commit()
}
