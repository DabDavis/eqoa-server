package database

// InventoryStore queries character inventory data (charinventory joined with itempattern).
type InventoryStore struct {
	db *SQLiteDB
}

// NewInventoryStore creates a new inventory store.
func NewInventoryStore(db *SQLiteDB) *InventoryStore {
	return &InventoryStore{db: db}
}

// CharInventoryEntry combines per-character inventory instance data with item pattern data.
type CharInventoryEntry struct {
	// Per-instance (from charinventory)
	ItemID    int // PK auto-increment, used as 'key' on wire
	StackLeft int
	RemainHP  int
	Charges   int
	EquipLoc  int // equipment slot (-1 = not equipped)
	Location  int // 0 = equipped/inventory, 1 = bank, etc.
	ListNum   int
	// Pattern data (from itempattern)
	Pattern ItemPatternRecord
}

// GetCharacterItems returns all inventory+bank items for a character, joined with patterns.
func (s *InventoryStore) GetCharacterItems(serverID uint32) ([]CharInventoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT c.itemid, c.stackleft, c.remainHP, c.remaincharge, c.equiploc, c.location, c.listnumber,
			p.patternid, p.itemcost, p.unk1, p.itemicon, p.unk2, p.equipslot, p.unk3,
			p.trade, p.rent, p.unk4, p.attacktype, p.weapondamage, p.unk5,
			p.levelreq, p.maxstack, p.maxhp, p.duration, p.classuse, p.raceuse,
			p.procanim, p.lore, p.unk6, p.craft, p.itemname, p.itemdesc, p.model, p.color,
			p.str, p.sta, p.agi, p.wis, p.dex, p.cha, p.intelligence,
			p.HPMAX, p.POWMAX, p.PoT, p.HoT, p.AC, p.PR, p.DR, p.FR, p.CR, p.LR, p.AR
		FROM charinventory c
		JOIN itempattern p ON c.patternid = p.patternid
		WHERE c.serverid = ?
		ORDER BY c.location, c.listnumber
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CharInventoryEntry
	for rows.Next() {
		var e CharInventoryEntry
		err := rows.Scan(
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
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// SetEquipLoc moves an item to an equipment slot, or to -1 to unequip it.
// SetInventoryOrder keeps the order of a character's pack (location 0) as the
// client's list holds it after the item menu's Arrange (0x003D): the rows of
// itemIDs first, in that order, then every other row of the pack in its old
// order, numbered from 0 (listnumber, which GetCharacterItems orders by, so
// the next MemoryDump lists them so). Scoped by serverID: an itemid not the
// character's is passed over.
func (s *InventoryStore) SetInventoryOrder(serverID uint32, itemIDs []int) error {
	rows, err := s.db.Query(`SELECT itemid FROM charinventory WHERE serverid = ? AND location = 0 ORDER BY listnumber, itemid`, serverID)
	if err != nil {
		return err
	}
	var all []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		all = append(all, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	have := make(map[int]bool, len(all))
	for _, id := range all {
		have[id] = true
	}
	order := make([]int, 0, len(all))
	placed := make(map[int]bool, len(all))
	for _, id := range itemIDs {
		if have[id] && !placed[id] {
			order = append(order, id)
			placed[id] = true
		}
	}
	for _, id := range all {
		if !placed[id] {
			order = append(order, id)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range order {
		if _, err := tx.Exec(`UPDATE charinventory SET listnumber = ? WHERE itemid = ? AND serverid = ? AND location = 0`, i, id, serverID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Scoped by serverID as well as itemid so a bad key cannot touch another
// character's inventory.
func (s *InventoryStore) SetEquipLoc(serverID uint32, itemID int, equipLoc int) error {
	_, err := s.db.Exec(
		`UPDATE charinventory SET equiploc = ? WHERE itemid = ? AND serverid = ?`,
		equipLoc, itemID, serverID)
	return err
}
