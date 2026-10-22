package database

import "fmt"

// NewCharacterRules is what a new character starts with: world.start_level
// and world.start_all_spells, set once at startup. Both are for testing.
type NewCharacterRules struct {
	Level     int  // 1 when unset; at most 60
	AllSpells bool // every spell its class has (classspells)
}

// SetNewCharacterRules sets what CreateNew gives; the GM panel changes it
// while the server runs.
func (s *SQLiteCharacterStore) SetNewCharacterRules(r NewCharacterRules) {
	s.rulesMu.Lock()
	s.newRules = r
	s.rulesMu.Unlock()
}

// NewCharacterRules is what CreateNew gives now.
func (s *SQLiteCharacterStore) NewCharacterRules() NewCharacterRules {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()
	return s.newRules
}

// startLevel is the level a new character starts at.
func (r NewCharacterRules) startLevel() int {
	return max(1, min(60, r.Level))
}

// CreateNew creates a character as a new one starts, whichever way it was
// made (the game's 0x002B or the account page): at the rules' level, whatever
// the record says, and with its class's spells when the rules say so. It
// returns how many spells it was given.
func (s *SQLiteCharacterStore) CreateNew(c *CharacterRecord) (int, error) {
	rules := s.NewCharacterRules()
	c.Level = rules.startLevel()
	if err := s.Create(c); err != nil {
		return 0, err
	}
	if !rules.AllSpells {
		return 0, nil
	}
	n, err := s.GrantClassSpells(c.ServerID, c.TClass)
	if err != nil {
		return n, fmt.Errorf("character %q made, its spells not: %w", c.CharName, err)
	}
	return n, nil
}

// GrantClassSpells puts every spell of a class (classspells) that the
// character lacks in its book: in level, then name order after what it has,
// off the bar, with no lockout and -1 at VIAbility+0x14, as a scribed spell
// goes in. It returns how many went in.
func (s *SQLiteCharacterStore) GrantClassSpells(serverID uint32, class int) (int, error) {
	rows, err := s.db.Query(`SELECT c.spellid FROM classspells c JOIN spellpattern p ON p.spellid = c.spellid
		WHERE c.class = ? AND c.spellid NOT IN (SELECT spellid FROM spells WHERE serverid = ?)
		ORDER BY p.abilitylvl, p.spellname, c.spellid`, class, serverID)
	if err != nil {
		return 0, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var next int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(addedorder) + 1, 0) FROM spells WHERE serverid = ? AND addedorder < 252`, serverID).Scan(&next); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	for i, id := range ids {
		if next+i >= 252 {
			break // the book shows display indices below 252
		}
		if _, err := tx.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
			VALUES (?, ?, ?, 0, -1, 0, -1)`, serverID, id, next+i); err != nil {
			tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return min(len(ids), 252-next), nil
}
