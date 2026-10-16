package database

import (
	"database/sql"
	"fmt"
)

// shop.go — what the merchant, bank and blacksmith windows read and write:
// an NPC's catalog (npcitem), single inventory rows, and the purse.

// itemPatternColumns is the itempattern half of every item row read here, in
// the order scanItemRow scans it.
const itemPatternColumns = `
	p.patternid, p.itemcost, p.unk1, p.itemicon, p.unk2, p.equipslot, p.unk3,
	p.trade, p.rent, p.unk4, p.attacktype, p.weapondamage, p.unk5,
	p.levelreq, p.maxstack, p.maxhp, p.duration, p.classuse, p.raceuse,
	p.procanim, p.lore, p.unk6, p.craft, p.itemname, p.itemdesc, p.model, p.color,
	p.str, p.sta, p.agi, p.wis, p.dex, p.cha, p.intelligence,
	p.HPMAX, p.POWMAX, p.PoT, p.HoT, p.AC, p.PR, p.DR, p.FR, p.CR, p.LR, p.AR`

// scanItemRow scans the instance columns (itemid, stackleft, remainHP,
// remaincharge, equiploc, location, listnumber) and then itemPatternColumns.
func scanItemRow(scan func(...any) error) (CharInventoryEntry, error) {
	var e CharInventoryEntry
	err := scan(
		&e.ItemID, &e.StackLeft, &e.RemainHP, &e.Charges, &e.EquipLoc, &e.Location, &e.ListNum,
		&e.Pattern.PatternID, &e.Pattern.ItemCost, &e.Pattern.Unk1, &e.Pattern.ItemIcon,
		&e.Pattern.Unk2, &e.Pattern.EquipSlot, &e.Pattern.Unk3,
		&e.Pattern.Trade, &e.Pattern.Rent, &e.Pattern.Unk4,
		&e.Pattern.AttackType, &e.Pattern.WeaponDamage, &e.Pattern.Unk5,
		&e.Pattern.LevelReq, &e.Pattern.MaxStack, &e.Pattern.MaxHP, &e.Pattern.Duration,
		&e.Pattern.ClassUse, &e.Pattern.RaceUse, &e.Pattern.ProcAnim,
		&e.Pattern.Lore, &e.Pattern.Unk6, &e.Pattern.Craft,
		&e.Pattern.ItemName, &e.Pattern.ItemDesc, &e.Pattern.Model, &e.Pattern.Color,
		&e.Pattern.Str, &e.Pattern.Sta, &e.Pattern.Agi, &e.Pattern.Wis,
		&e.Pattern.Dex, &e.Pattern.Cha, &e.Pattern.Intelligence,
		&e.Pattern.HPMAX, &e.Pattern.POWMAX, &e.Pattern.PoT, &e.Pattern.HoT, &e.Pattern.AC,
		&e.Pattern.PR, &e.Pattern.DR, &e.Pattern.FR, &e.Pattern.CR, &e.Pattern.LR, &e.Pattern.AR,
	)
	return e, err
}

// GetNPCItems returns an NPC's merchant catalog (npcitem joined with
// itempattern) in list order — the order 0x00B7 writes it, so a 0x004A's
// list index names a row of it.
func (s *InventoryStore) GetNPCItems(npcID int) ([]CharInventoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT n.itemid, n.stackleft, n.remainHP, n.remaincharge, n.equiploc, n.location, n.listnumber,
		`+itemPatternColumns+`
		FROM npcitem n
		JOIN itempattern p ON n.patternid = p.patternid
		WHERE n.npcid = ?
		ORDER BY n.listnumber, n.itemid
	`, npcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CharInventoryEntry
	for rows.Next() {
		e, err := scanItemRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// GetCharItem returns one of a character's item rows with its pattern.
func (s *InventoryStore) GetCharItem(serverID uint32, itemID int) (CharInventoryEntry, error) {
	row := s.db.QueryRow(`
		SELECT c.itemid, c.stackleft, c.remainHP, c.remaincharge, c.equiploc, c.location, c.listnumber,
		`+itemPatternColumns+`
		FROM charinventory c
		JOIN itempattern p ON c.patternid = p.patternid
		WHERE c.serverid = ? AND c.itemid = ?
	`, serverID, itemID)
	e, err := scanItemRow(row.Scan)
	if err == sql.ErrNoRows {
		return e, fmt.Errorf("character %d has no item %d", serverID, itemID)
	}
	return e, err
}

// AddCharItem gives a character a new item row at the end of the list at
// location (0 the inventory, 1 the bank), unequipped, and returns its itemid.
func (s *InventoryStore) AddCharItem(serverID uint32, patternID, stack, remainHP, charges, location int) (int, error) {
	res, err := s.db.Exec(`
		INSERT INTO charinventory (serverid, stackleft, remainHP, remaincharge, patternid, equiploc, location, listnumber)
		VALUES (?, ?, ?, ?, ?, -1, ?,
			(SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?))
	`, serverID, stack, remainHP, charges, patternID, location, serverID, location)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// SetItemStack sets a row's stack. Scoped by serverID as SetEquipLoc is.
func (s *InventoryStore) SetItemStack(serverID uint32, itemID, stack int) error {
	return s.execOne(`UPDATE charinventory SET stackleft = ? WHERE itemid = ? AND serverid = ?`,
		stack, itemID, serverID)
}

// SetItemRemainHP sets a row's condition (a repair).
func (s *InventoryStore) SetItemRemainHP(serverID uint32, itemID, hp int) error {
	return s.execOne(`UPDATE charinventory SET remainHP = ? WHERE itemid = ? AND serverid = ?`,
		hp, itemID, serverID)
}

// DeleteCharItem removes a row (sold, or the last of a stack moved).
func (s *InventoryStore) DeleteCharItem(serverID uint32, itemID int) error {
	return s.execOne(`DELETE FROM charinventory WHERE itemid = ? AND serverid = ?`, itemID, serverID)
}

// MoveCharItem moves a row to the end of the list at location, unequipped
// (an item crosses between the inventory and the bank whole).
func (s *InventoryStore) MoveCharItem(serverID uint32, itemID, location int) error {
	return s.execOne(`
		UPDATE charinventory SET location = ?, equiploc = -1,
			listnumber = (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM charinventory WHERE serverid = ? AND location = ?)
		WHERE itemid = ? AND serverid = ?
	`, location, serverID, location, itemID, serverID)
}

// SetTunar stores a character's purse and bank tunar.
func (s *InventoryStore) SetTunar(serverID uint32, tunar, bankTunar int) error {
	return s.execOne(`UPDATE characters SET tunar = ?, bankTunar = ? WHERE serverid = ?`,
		tunar, bankTunar, serverID)
}

// execOne runs an UPDATE or DELETE that must touch exactly one row.
func (s *InventoryStore) execOne(query string, args ...any) error {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return fmt.Errorf("%d rows changed, want 1", n)
	}
	return nil
}
