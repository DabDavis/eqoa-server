package database

import "fmt"

// The HUD's item quick slots and weapon sets, as SOE's live client has them
// (claude-notes/live-toolbars-gear-sets-20261024.md): four quick slots, each
// holding one inventory item, and four named weapon sets, each the main hand
// and the off hand as they were worn when the set was prepared.

// QuickSlots is how many item quick slots there are (the HUD's third menu).
const QuickSlots = 4

// GearSets is how many weapon sets there are (the live client's fourth HUD
// menu, with "bare hands" after them).
const GearSets = 4

// GearSet is one weapon set: its name and the two hands' items, each by its
// charinventory.itemid and its itempattern.patternid (the client finds a set's
// items by pattern). MainItem -1 is an empty set.
type GearSet struct {
	Name        string
	MainItem    int
	MainPattern int
	OffItem     int
	OffPattern  int
}

// EmptyGearSet is a set never prepared.
var EmptyGearSet = GearSet{MainItem: -1, MainPattern: -1, OffItem: -1, OffPattern: -1}

// QuickSlotsOf returns each prepared item's quick slot, by charinventory.itemid.
func (s *InventoryStore) QuickSlotsOf(serverID uint32) (map[int]int, error) {
	rows, err := s.db.Query(`SELECT pos, itemid FROM quickslots WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var pos, itemID int
		if err := rows.Scan(&pos, &itemID); err != nil {
			return nil, err
		}
		if pos >= 0 && pos < QuickSlots {
			out[itemID] = pos
		}
	}
	return out, rows.Err()
}

// SetQuickSlot puts an item on quick slot pos: whatever was there comes off it,
// and the item comes off any other slot (an item is on one slot at most, and a
// slot holds one item, as Msg000A keeps them).
func (s *InventoryStore) SetQuickSlot(serverID uint32, pos, itemID int) error {
	if pos < 0 || pos >= QuickSlots {
		return fmt.Errorf("quick slot %d out of range", pos)
	}
	if _, err := s.db.Exec(`DELETE FROM quickslots WHERE serverid = ? AND (pos = ? OR itemid = ?)`, serverID, pos, itemID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO quickslots (serverid, pos, itemid) VALUES (?, ?, ?)`, serverID, pos, itemID)
	return err
}

// dropQuickSlot takes an item off its quick slot (it left the inventory).
func (s *InventoryStore) dropQuickSlot(serverID uint32, itemID int) error {
	_, err := s.db.Exec(`DELETE FROM quickslots WHERE serverid = ? AND itemid = ?`, serverID, itemID)
	return err
}

// GearSetsOf returns the character's four weapon sets, empty where none was
// prepared.
func (s *InventoryStore) GearSetsOf(serverID uint32) ([GearSets]GearSet, error) {
	var out [GearSets]GearSet
	for i := range out {
		out[i] = EmptyGearSet
	}
	rows, err := s.db.Query(`SELECT slot, name, mainitem, mainpattern, offitem, offpattern FROM gearsets WHERE serverid = ?`, serverID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var slot int
		var g GearSet
		if err := rows.Scan(&slot, &g.Name, &g.MainItem, &g.MainPattern, &g.OffItem, &g.OffPattern); err != nil {
			return out, err
		}
		if slot >= 0 && slot < GearSets {
			out[slot] = g
		}
	}
	return out, rows.Err()
}

// SetGearSet stores weapon set slot.
func (s *InventoryStore) SetGearSet(serverID uint32, slot int, g GearSet) error {
	if slot < 0 || slot >= GearSets {
		return fmt.Errorf("weapon set %d out of range", slot)
	}
	_, err := s.db.Exec(`
		INSERT INTO gearsets (serverid, slot, name, mainitem, mainpattern, offitem, offpattern)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (serverid, slot) DO UPDATE SET name = excluded.name,
			mainitem = excluded.mainitem, mainpattern = excluded.mainpattern,
			offitem = excluded.offitem, offpattern = excluded.offpattern
	`, serverID, slot, g.Name, g.MainItem, g.MainPattern, g.OffItem, g.OffPattern)
	return err
}
