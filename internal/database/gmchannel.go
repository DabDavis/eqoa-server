package database

import (
	"database/sql"
	"errors"
	"strings"
)

// What the GM's TEST menu reads and writes (handlers/gmchannel.go).

// ItemPatternByID is the item of that patternid, and whether there is one:
// a GM's Summon Item and Give Item name an item by its number.
func (s *InventoryStore) ItemPatternByID(id int) (ItemPatternRecord, bool, error) {
	p, err := NewItemPatternStore(s.db).GetItemPattern(id)
	if errors.Is(err, sql.ErrNoRows) {
		return ItemPatternRecord{}, false, nil
	}
	if err != nil {
		return ItemPatternRecord{}, false, err
	}
	return *p, true, nil
}

// ErrNameTaken is a rename to a name another character has.
var ErrNameTaken = errors.New("that name is taken")

// Rename gives a character a new name (a GM's Change Name): refused with
// ErrNameTaken when another character has it, without case.
func (s *SQLiteCharacterStore) Rename(serverID uint32, name string) error {
	name = strings.TrimSpace(name)
	var other int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM characters WHERE LOWER(charName) = LOWER(?) AND serverid != ?`, name, serverID).Scan(&other)
	if err != nil {
		return err
	}
	if other > 0 {
		return ErrNameTaken
	}
	res, err := s.db.Exec(`UPDATE characters SET charName = ? WHERE serverid = ?`, name, serverID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n != 1 {
		return ErrCharacterNotFound
	}
	return nil
}
