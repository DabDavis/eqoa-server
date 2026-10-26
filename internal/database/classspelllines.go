package database

// ClassSpellLine is one of a class's spells (classspells) with what the
// world needs to cast it on its own: its ability record's level, scope,
// name and description, and its spelleffect line and visual ("" and 0 for a
// spell with no row there).
type ClassSpellLine struct {
	SpellID int
	Level   int
	Scope   int
	Name    string
	Desc    string
	Line    string
	FX      uint32
}

// ClassSpellLines is every spell a character of a class gets by level (not a
// mastery's, an epic's or a master class's: Granted), by level: what a
// necromancer's pet taps with (entity/pettap.go).
func (db *SQLiteDB) ClassSpellLines(class int) ([]ClassSpellLine, error) {
	rows, err := db.Query(`
		SELECT sp.spellid, sp.abilitylvl, sp.scope, sp.spellname, COALESCE(sp.spelldesc, ''),
			COALESCE(se.line, ''), COALESCE(se.fxdict, 0), COALESCE(cs.source, '')
		FROM classspells cs
		JOIN spellpattern sp ON sp.spellid = cs.spellid
		LEFT JOIN spelleffect se ON se.spellid = sp.spellid
		WHERE cs.class = ?
		ORDER BY sp.abilitylvl, sp.spellid`, class)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClassSpellLine
	for rows.Next() {
		var l ClassSpellLine
		var fx int64
		var source string
		if err := rows.Scan(&l.SpellID, &l.Level, &l.Scope, &l.Name, &l.Desc, &l.Line, &fx, &source); err != nil {
			return nil, err
		}
		if !Granted(source) {
			continue
		}
		l.FX = uint32(fx)
		out = append(out, l)
	}
	return out, rows.Err()
}
