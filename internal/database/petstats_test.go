package database

import (
	"strings"
	"testing"
)

// petstats as the server reads it: a row whole, its stances and abilities
// read (a spell pinned by id, a heal of either), the rows it cannot use left
// out and said; a pinned id found with TemplateSpellByID. Made-up numbers.
func TestPetStatsRows(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/pets.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (902100, 21, 0, 0, 16880, 1, 10, 0, 0, 1, 12, 0, 'Pet Strike', '')`,
		`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (902100, 'Target, Decrease Hitpoints by 84', 0, 'test')`,
		`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, stances, ac, str, intelligence, hp, ar, dr,
			cha_max, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, keep_template, source) VALUES
			(717, '002F', ' Fire Elementalkin ', 3000000000, 40, 'Melee', 'Passive, Defensive', 1200, 59, 11, 2640, 20, 25,
			400, 480, 'Slash', 440, 'pierce',
			'[{"kind":"nuke","spell_id":902100,"every":6},{"kind":"heal","spell_id":902100,"target":"either","every":30}]', 1, 'eqoaemu')`,
		`INSERT INTO petstats (spellid, name, type) VALUES (2, 'odd type', 'wizard')`,
		`INSERT INTO petstats (spellid, name, stances) VALUES (3, 'odd stance', 'berserk')`,
		`INSERT INTO petstats (spellid, name, wpn1_type) VALUES (4, 'odd weapon', 'axe')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (5, 'two pins', '[{"kind":"nuke","spell":"Pet Strike","spell_id":902100,"every":6}]')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (6, 'either buff', '[{"kind":"buff","spell_id":902100,"target":"either","every":6}]')`,
		`INSERT INTO petstats (spellid, name, hp) VALUES (7, 'negative', -1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	rows, bad, err := db.PetStatsRows()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := rows[717]
	if !ok || len(rows) != 1 {
		t.Fatalf("rows %v: want 717 alone", rows)
	}
	if p.Name != "Fire Elementalkin" || p.Model != 3000000000 || p.Level != 40 || p.Type != PetTypeMelee || p.AC != 1200 ||
		p.Str != 59 || p.Int != 11 || p.HP != 2640 || p.AR != 20 || p.DR != 25 || p.ChaMax != 400 || !p.KeepTemplate {
		t.Errorf("row read %+v", p)
	}
	if p.Wpn1Dmg != 480 || p.Wpn1Type != "slash" || p.Wpn2Dmg != 440 || p.Wpn2Type != "pierce" {
		t.Errorf("weapons %d %q %d %q", p.Wpn1Dmg, p.Wpn1Type, p.Wpn2Dmg, p.Wpn2Type)
	}
	if !p.TakesStance(1) || !p.TakesStance(2) || p.TakesStance(3) || p.TakesStance(4) {
		t.Errorf("stances %v: passive and defensive alone", p.Stances)
	}
	if len(p.Abilities) != 2 || p.Abilities[0].SpellID != 902100 || p.Abilities[1].TargetOr() != AbilityOnEither {
		t.Errorf("abilities %+v", p.Abilities)
	}
	if len(bad) != 6 {
		t.Errorf("bad %q: want the six rows it cannot use", bad)
	}
	for _, want := range []string{"wizard", "berserk", "axe", "by name and by id", "either is a heal's", "hp -1"} {
		found := false
		for _, b := range bad {
			found = found || strings.Contains(b, want)
		}
		if !found {
			t.Errorf("no problem saying %q in %q", want, bad)
		}
	}
	s, ok, err := db.TemplateSpellByID(902100)
	if err != nil || !ok || s.Pattern.SpellName != "Pet Strike" || s.Line != "Target, Decrease Hitpoints by 84" {
		t.Errorf("TemplateSpellByID: %+v %v %v", s, ok, err)
	}
	if _, ok, err := db.TemplateSpellByID(902101); ok || err != nil {
		t.Errorf("a spell not there: %v %v", ok, err)
	}
	var none PetStats
	if !none.TakesStance(4) {
		t.Error("a row of no stances takes all four")
	}
}
