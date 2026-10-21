package database

import "database/sql"

// SpellbookStore queries character spell data (spells joined with spellpattern).
type SpellbookStore struct {
	db *SQLiteDB
}

// NewSpellbookStore creates a new spellbook store.
func NewSpellbookStore(db *SQLiteDB) *SpellbookStore {
	return &SpellbookStore{db: db}
}

// CharSpellEntry combines per-character spell instance data with spell pattern data.
type CharSpellEntry struct {
	// Per-instance (from spells table)
	AddedOrder    int
	OnHotBar      int
	WhereOnHotBar int
	Unk1          int
	ShowHide      int
	// Pattern data (from spellpattern)
	Pattern SpellPatternRecord
}

// GetCharacterSpells returns all spells for a character, joined with patterns.
func (s *SpellbookStore) GetCharacterSpells(serverID uint32) ([]CharSpellEntry, error) {
	rows, err := s.db.Query(`
		SELECT s.addedorder, s.onHotBar, s.whereonBar, s.unk1, s.showhide,
			sp.spellid, sp.abilitylvl, sp.unk2, sp.unk3, sp.range, sp.casttime, sp.power,
			sp.iconColor, sp.icon, sp.scope, sp.recast, sp.eqprequire,
			sp.spellname, sp.spelldesc
		FROM spells s
		JOIN spellpattern sp ON s.spellid = sp.spellid
		WHERE s.serverid = ?
		ORDER BY s.addedorder
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spells []CharSpellEntry
	for rows.Next() {
		var e CharSpellEntry
		err := rows.Scan(
			&e.AddedOrder, &e.OnHotBar, &e.WhereOnHotBar, &e.Unk1, &e.ShowHide,
			&e.Pattern.SpellID, &e.Pattern.AbilityLvl, &e.Pattern.Unk2, &e.Pattern.Unk3,
			&e.Pattern.Range, &e.Pattern.CastTime, &e.Pattern.Power,
			&e.Pattern.IconColor, &e.Pattern.Icon, &e.Pattern.Scope,
			&e.Pattern.Recast, &e.Pattern.EqpRequire,
			&e.Pattern.SpellName, &e.Pattern.SpellDesc,
		)
		if err != nil {
			return nil, err
		}
		spells = append(spells, e)
	}
	return spells, rows.Err()
}

// SpellRow is one row of a character's spellbook as the bar and the order
// keep it.
type SpellRow struct {
	SpellID       int
	AddedOrder    int
	OnHotBar      int
	WhereOnHotBar int
}

// GetSpellRows returns the character's spellbook rows.
func (s *SpellbookStore) GetSpellRows(serverID uint32) ([]SpellRow, error) {
	rows, err := s.db.Query(`SELECT spellid, addedorder, onHotBar, whereonBar FROM spells WHERE serverid = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpellRow
	for rows.Next() {
		var r SpellRow
		if err := rows.Scan(&r.SpellID, &r.AddedOrder, &r.OnHotBar, &r.WhereOnHotBar); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetSpellRow writes one row's order and bar slot back.
func (s *SpellbookStore) SetSpellRow(serverID uint32, r SpellRow) error {
	_, err := s.db.Exec(`UPDATE spells SET addedorder = ?, onHotBar = ?, whereonBar = ? WHERE serverid = ? AND spellid = ?`,
		r.AddedOrder, r.OnHotBar, r.WhereOnHotBar, serverID, r.SpellID)
	return err
}

// SpellName is one spell pattern's id, name and level.
type SpellName struct {
	SpellID int
	Name    string
	Level   int
}

// SpellNames returns every spell pattern's id, name and level, in id order:
// a scroll names the spell it teaches by its own name, and nothing else.
func (s *SpellbookStore) SpellNames() ([]SpellName, error) {
	rows, err := s.db.Query(`SELECT spellid, spellname, abilitylvl FROM spellpattern ORDER BY spellid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpellName
	for rows.Next() {
		var n SpellName
		if err := rows.Scan(&n.SpellID, &n.Name, &n.Level); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddSpell puts a spell in a character's book at display index order, off
// the bar, with no lockout and -1 at VIAbility+0x14: the record Sandstorm
// sends for a scroll just scribed.
func (s *SpellbookStore) AddSpell(serverID uint32, spellID, order int) error {
	_, err := s.db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
		VALUES (?, ?, ?, 0, -1, 0, -1)`, serverID, spellID, order)
	return err
}

// SpellEffect is a spell's spelleffect row: the line saying what it does
// and the visual Sandstorm names for it (0 when none was recorded). ok is
// false for a spell with no row.
func (s *SpellbookStore) SpellEffect(spellID int) (line string, fx uint32, ok bool, err error) {
	var dict int64
	err = s.db.QueryRow(`SELECT line, fxdict FROM spelleffect WHERE spellid = ?`, spellID).Scan(&line, &dict)
	if err == sql.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return line, uint32(dict), true, nil
}
