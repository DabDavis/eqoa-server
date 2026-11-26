package database

import (
	"database/sql"
	"errors"
)

// legacyforms: an ability's older form, owned through a class mastery
// (Heirloom petition 3184: "the ability to return abilities back to older
// versions with CMs, like when the paladin 51 was a weapon proc and Fervor was
// an HP proc damage shield"; the owner approved the mechanism). A row binds an
// ability of the book (spellid, its spellpattern) to the spell it casts as
// instead (legacy_spellid, a spellpattern row of its own with its spelleffect)
// for a character that owns the mastery rank mastery_id (cmcatalog.id,
// charcms): the book keeps its ability where it stands, on its bar, and a
// cast of it is the legacy spell's (handlers/cast.go). Removing or resetting
// the rank gives the ability its present form again at its next cast. note
// says what the older form was and where its figures come from; enabled 0
// turns a row off (the rank is then nothing). source: gm, or the seeding
// tool's (tools/legacy_forms_20261126.py, whose --undo deletes its rows).
const LegacyFormsSchema = `
	CREATE TABLE IF NOT EXISTS legacyforms (
		spellid        INTEGER NOT NULL,
		legacy_spellid INTEGER NOT NULL,
		mastery_id     INTEGER NOT NULL,
		note           TEXT NOT NULL DEFAULT '',
		enabled        INTEGER NOT NULL DEFAULT 1,
		source         TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (spellid, mastery_id)
	);
	CREATE INDEX IF NOT EXISTS idx_legacyforms_mastery ON legacyforms(mastery_id);`

// LegacyForm is one row of legacyforms: the ability, the spell it casts as,
// and the mastery rank that makes it so.
type LegacyForm struct {
	SpellID, LegacySpellID, MasteryID int
	Note                              string
}

// LegacyFormsOf is the enabled older forms of the ability spellID, by mastery
// rank order (a character owning two such ranks casts the first's).
func (s *SpellbookStore) LegacyFormsOf(spellID int) ([]LegacyForm, error) {
	rows, err := s.db.Query(`SELECT spellid, legacy_spellid, mastery_id, note FROM legacyforms
		WHERE spellid = ? AND enabled <> 0 ORDER BY mastery_id`, spellID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LegacyForm
	for rows.Next() {
		var f LegacyForm
		if err := rows.Scan(&f.SpellID, &f.LegacySpellID, &f.MasteryID, &f.Note); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// IsLegacyMastery is whether the mastery rank id gives an older form of some
// ability (an enabled legacyforms row names it).
func (s *SpellbookStore) IsLegacyMastery(id int) (bool, error) {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM legacyforms WHERE mastery_id = ? AND enabled <> 0 LIMIT 1`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
