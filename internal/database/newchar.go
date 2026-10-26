package database

import (
	"database/sql"
	"fmt"
	"log"
)

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
// the record says, in its home city (startPlace), bound there, and with its
// class's first abilities, or all its class's spells when the rules say so.
// It returns how many spells it was given.
func (s *SQLiteCharacterStore) CreateNew(c *CharacterRecord) (int, error) {
	rules := s.NewCharacterRules()
	c.Level = rules.startLevel()
	city, placed := "", false
	if c.X == 0 && c.Y == 0 && c.Z == 0 {
		city, placed = s.startPlace(c)
	}
	if err := s.Create(c); err != nil {
		return 0, err
	}
	if placed {
		// a new character is bound where it starts until a spiritmaster
		// binds it: Return Home and a death take it home, not to Freeport
		if err := s.SaveBind(c.ServerID, BindRow{World: c.World, X: c.X, Y: c.Y, Z: c.Z,
			Facing: c.Facing, Spiritmaster: city}); err != nil {
			return 0, fmt.Errorf("character %q made, its bind point not: %w", c.CharName, err)
		}
	}
	var n int
	var err error
	if rules.AllSpells {
		n, err = s.GrantClassSpells(c.ServerID, c.TClass)
	} else {
		n, err = s.grantSpells(c.ServerID, StartingAbilities(c.TClass))
	}
	if err != nil {
		return n, fmt.Errorf("character %q made, its spells not: %w", c.CharName, err)
	}
	return n, nil
}

// raceFroglok is the race slot the froglok takes (handlers.RaceFroglok): a
// human of the east in all but its body.
const raceFroglok = 10

// startPlace puts a new character on its home city's spot, defaultcharacter's
// row for its race, class and city (humType: a human's east 1 or west 2, 0
// for the others), and names the city. defaultcharacter holds the live game's
// spots: Sandstorm's new characters stand exactly on them (F3 pass 17: a new
// Freeport warrior on the human-east warrior's row, a new Surefall ranger on
// its row), each a few steps from the class's first quest giver. A class the
// table lacks takes a row of its race; a human of no city the east's; false
// when the race has no row at all (the record keeps the origin, and the login
// spawn takes it).
func (s *SQLiteCharacterStore) startPlace(c *CharacterRecord) (string, bool) {
	race, hum := c.Race, c.HumType
	if race == raceFroglok {
		race, hum = 0, 1
	}
	var x, y, z, facing float64
	var world, rowHum, rowClass int
	err := s.db.QueryRow(`SELECT x, y, COALESCE(z, 0), facing, COALESCE(world, 0), humType, tclass
		FROM defaultcharacter WHERE race = ?
		ORDER BY tclass = ? DESC, humType = ? DESC, humType, tclass LIMIT 1`, race, c.TClass, hum).
		Scan(&x, &y, &z, &facing, &world, &rowHum, &rowClass)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("CreateNew: the start place of race %d class %d: %v", c.Race, c.TClass, err)
		}
		return "", false
	}
	c.X, c.Y, c.Z = float32(x), float32(y), float32(z)
	c.Facing, c.World = float32(facing), world
	return HomeCity(race, rowHum, rowClass), true
}

// HomeCity names the city a race (with its human city, humType) and class
// starts in, as EQOA-Data's starting cities list has them: the elves'
// rangers, bards, rogues and druids in Tethelin, the rest in Fayspire; a
// western human's rangers and druids in Surefall Glade.
func HomeCity(race, humType, class int) string {
	switch race {
	case 0:
		if humType == 2 {
			if class == 1 || class == 7 {
				return "Surefall Glade"
			}
			return "Qeynos"
		}
		return "Freeport"
	case 1:
		if class == 1 || class == 5 || class == 6 || class == 7 {
			return "Tethelin"
		}
		return "Fayspire"
	case raceFroglok:
		return "Freeport"
	}
	cities := [...]string{2: "Neriak", 3: "Klik'Anon", 4: "Moradhim", 5: "Grobb", 6: "Halas",
		7: "Rivervale", 8: "Highbourne", 9: "Oggok"}
	if race > 0 && race < len(cities) {
		return cities[race]
	}
	return "its home city"
}

// returnHome is Return Home as Sandstorm numbers it, every fresh character's
// second ability.
const returnHome = 1623

// startingAbilities are what a fresh character of each class has on
// Sandstorm, besides Return Home: one ability. Measured on its fresh
// characters (eqoa harvest dumps and questrec16's warrior, F3 pass 17):
// WAR Quick Strike, RAN Thistlecoat, PAL Divine Strike, SK Chilling Aura,
// MNK Mend (a level 5's book), RGE Hide, DRD and CL Minor Healing, ENC Lull,
// ALC Unstable Mixture. Not measured, ours: SHA Minor Healing (the other
// two priests'), BRD Nimble Strike (the melee classes' strike), MAG Burst of
// Frost and NEC Disease Cloud (their first attack spell, as the alchemist's
// Unstable Mixture is), WIZ Minor Fire Storm (first in a level 6 wizard's
// book).
var startingAbilities = [15]int{
	0: 1517, 1: 1096, 2: 1006, 3: 1420, 4: 785, 5: 142, 6: 1213, 7: 457,
	8: 457, 9: 457, 10: 631, 11: 875, 12: 231, 13: 1624, 14: 4,
}

// StartingAbilities are a fresh character's abilities: its class's first
// one, then Return Home, in that order in its book.
func StartingAbilities(class int) []int {
	if class < 0 || class >= len(startingAbilities) {
		return []int{returnHome}
	}
	return []int{startingAbilities[class], returnHome}
}

// grantSpells puts the given spells the character lacks in its book in that
// order after what it has, as GrantClassSpells does; a spell with no
// spellpattern row is passed over (the client would show it blank).
func (s *SQLiteCharacterStore) grantSpells(serverID uint32, spells []int) (int, error) {
	var ids []int
	for _, id := range spells {
		var known int
		if err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM spellpattern WHERE spellid = ?) -
			(SELECT COUNT(*) FROM spells WHERE serverid = ? AND spellid = ?)`, id, serverID, id).Scan(&known); err != nil {
			return 0, err
		}
		if known > 0 {
			ids = append(ids, id)
		} else {
			log.Printf("CreateNew: spell %d is not in spellpattern or already known; not given", id)
		}
	}
	return s.addToBook(serverID, ids)
}

// GrantClassSpells puts every spell of a class (classspells) that the
// character lacks in its book: in level, then name order after what it has,
// off the bar, with no lockout and -1 at VIAbility+0x14, as a scribed spell
// goes in. A were form or a were's ability is not a class's (IsWereAbility):
// it is never given this way. It returns how many went in.
func (s *SQLiteCharacterStore) GrantClassSpells(serverID uint32, class int) (int, error) {
	rows, err := s.db.Query(`SELECT c.spellid, p.spellname, COALESCE(p.spelldesc, '') FROM classspells c
		JOIN spellpattern p ON p.spellid = c.spellid
		WHERE c.class = ? AND c.spellid NOT IN (SELECT spellid FROM spells WHERE serverid = ?)
		ORDER BY p.abilitylvl, p.spellname, c.spellid`, class, serverID)
	if err != nil {
		return 0, err
	}
	var ids []int
	for rows.Next() {
		var id int
		var name, desc string
		if err := rows.Scan(&id, &name, &desc); err != nil {
			rows.Close()
			return 0, err
		}
		if !IsWereAbility(name, desc) {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return s.addToBook(serverID, ids)
}

// addToBook writes spells into a character's book after what it holds, in the
// order given, as a scribed spell goes in: off the bar, no lockout, -1 at
// VIAbility+0x14. It returns how many went in (the book shows display indices
// below 252).
func (s *SQLiteCharacterStore) addToBook(serverID uint32, ids []int) (int, error) {
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
