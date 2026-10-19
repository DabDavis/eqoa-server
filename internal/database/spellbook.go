package database

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
