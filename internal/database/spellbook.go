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
