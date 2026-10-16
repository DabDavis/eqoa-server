package database

// SpellPatternRecord represents a row from the spellpattern table.
type SpellPatternRecord struct {
	SpellID    int
	AbilityLvl int
	Unk2       int
	Unk3       int
	Range      int
	CastTime   int
	Power      int
	IconColor  int64
	Icon       int64
	Scope      int
	Recast     int
	EqpRequire int
	SpellName  string
	SpellDesc  string
}

// SpellPatternStore queries the spellpattern table.
type SpellPatternStore struct {
	db *SQLiteDB
}

// NewSpellPatternStore creates a new spell pattern store.
func NewSpellPatternStore(db *SQLiteDB) *SpellPatternStore {
	return &SpellPatternStore{db: db}
}

// GetSpellPattern retrieves a spell pattern by spellid.
func (s *SpellPatternStore) GetSpellPattern(spellID int) (*SpellPatternRecord, error) {
	row := s.db.QueryRow(`
		SELECT spellid, abilitylvl, unk2, unk3, range, casttime, power,
			iconColor, icon, scope, recast, eqprequire, spellname, spelldesc
		FROM spellpattern WHERE spellid = ?
	`, spellID)

	var sp SpellPatternRecord
	err := row.Scan(
		&sp.SpellID, &sp.AbilityLvl, &sp.Unk2, &sp.Unk3,
		&sp.Range, &sp.CastTime, &sp.Power,
		&sp.IconColor, &sp.Icon, &sp.Scope, &sp.Recast, &sp.EqpRequire,
		&sp.SpellName, &sp.SpellDesc,
	)
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// UpsertSpellPattern inserts or replaces a spell pattern by spellid.
func (s *SpellPatternStore) UpsertSpellPattern(r *SpellPatternRecord) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO spellpattern (
			spellid, abilitylvl, unk2, unk3, range, casttime, power,
			iconColor, icon, scope, recast, eqprequire, spellname, spelldesc
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`,
		r.SpellID, r.AbilityLvl, r.Unk2, r.Unk3,
		r.Range, r.CastTime, r.Power,
		r.IconColor, r.Icon, r.Scope, r.Recast, r.EqpRequire,
		r.SpellName, r.SpellDesc,
	)
	return err
}

// Count returns the total number of spell patterns.
func (s *SpellPatternStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM spellpattern`).Scan(&count)
	return count, err
}
