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
			cha_max, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, keep_template, size, source) VALUES
			(717, 'T001', ' Test Flame ', 3000000000, 38, 'Melee', 'Passive, Defensive', 1100, 55, 12, 2500, 20, 25,
			400, 450, 'Slash', 400, 'pierce',
			'[{"kind":"nuke","spell_id":902100,"every":6},{"kind":"heal","spell_id":902100,"target":"either","every":30}]', 1, 3.0, 'eqoaemu')`,
		`INSERT INTO petstats (spellid, name, type) VALUES (2, 'odd type', 'wizard')`,
		`INSERT INTO petstats (spellid, name, stances) VALUES (3, 'odd stance', 'berserk')`,
		`INSERT INTO petstats (spellid, name, wpn1_type) VALUES (4, 'odd weapon', 'axe')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (5, 'two pins', '[{"kind":"nuke","spell":"Pet Strike","spell_id":902100,"every":6}]')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (6, 'either buff', '[{"kind":"buff","spell_id":902100,"target":"either","every":6}]')`,
		`INSERT INTO petstats (spellid, name, hp) VALUES (7, 'negative', -1)`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (8, 'group buff', '[{"kind":"buff","spell_id":902100,"target":"group","every":6}]')`,
		`INSERT INTO petstats (spellid, name, abilities) VALUES (9, 'group heal', '[{"kind":"heal","spell_id":902100,"target":"group","below":100,"every":6}]')`,
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
	if !ok || len(rows) != 2 {
		t.Fatalf("rows %v: want 717 and 9 alone", rows)
	}
	// petition 1495: a heal of the pet's whole side (target group), a heal's alone
	if g, ok := rows[9]; !ok || len(g.Abilities) != 1 || g.Abilities[0].TargetOr() != AbilityOnGroup || g.Abilities[0].BelowOr() != 100 {
		t.Errorf("the group heal's row: %+v", g)
	}
	if p.Name != "Test Flame" || p.Model != 3000000000 || p.Level != 38 || p.Type != PetTypeMelee || p.AC != 1100 ||
		p.Str != 55 || p.Int != 12 || p.HP != 2500 || p.AR != 20 || p.DR != 25 || p.ChaMax != 400 || !p.KeepTemplate || p.Size != 3 {
		t.Errorf("row read %+v", p)
	}
	if p.Wpn1Dmg != 450 || p.Wpn1Type != "slash" || p.Wpn2Dmg != 400 || p.Wpn2Type != "pierce" {
		t.Errorf("weapons %d %q %d %q", p.Wpn1Dmg, p.Wpn1Type, p.Wpn2Dmg, p.Wpn2Type)
	}
	if !p.TakesStance(1) || !p.TakesStance(2) || p.TakesStance(3) || p.TakesStance(4) {
		t.Errorf("stances %v: passive and defensive alone", p.Stances)
	}
	if len(p.Abilities) != 2 || p.Abilities[0].SpellID != 902100 || p.Abilities[1].TargetOr() != AbilityOnEither {
		t.Errorf("abilities %+v", p.Abilities)
	}
	if len(bad) != 7 {
		t.Errorf("bad %q: want the seven rows it cannot use", bad)
	}
	for _, want := range []string{"wizard", "berserk", "axe", "by name and by id", "either is a heal's", "group is a heal's", "hp -1"} {
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

// petstats' melee_pct (Live petition 5555): read as written, 0 as 100, and a table made before it gains it at the
// next open, every row at 100.
func TestPetStatsMeleePct(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`DROP TABLE petstats`,
		`CREATE TABLE petstats (spellid INTEGER PRIMARY KEY, sheet_pet_id TEXT NOT NULL DEFAULT '', name TEXT NOT NULL DEFAULT '',
			model INTEGER NOT NULL DEFAULT 0, primary_model INTEGER NOT NULL DEFAULT 0, secondary_model INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 0, type TEXT NOT NULL DEFAULT '', element TEXT NOT NULL DEFAULT '', stances TEXT NOT NULL DEFAULT '',
			ac INTEGER NOT NULL DEFAULT 0, str INTEGER NOT NULL DEFAULT 0, sta INTEGER NOT NULL DEFAULT 0, agi INTEGER NOT NULL DEFAULT 0,
			dex INTEGER NOT NULL DEFAULT 0, wis INTEGER NOT NULL DEFAULT 0, intelligence INTEGER NOT NULL DEFAULT 0, cha INTEGER NOT NULL DEFAULT 0,
			hp INTEGER NOT NULL DEFAULT 0, power INTEGER NOT NULL DEFAULT 0, hot INTEGER NOT NULL DEFAULT 0, pot INTEGER NOT NULL DEFAULT 0,
			ar INTEGER NOT NULL DEFAULT 0, cr INTEGER NOT NULL DEFAULT 0, lr INTEGER NOT NULL DEFAULT 0, fr INTEGER NOT NULL DEFAULT 0,
			pr INTEGER NOT NULL DEFAULT 0, dr INTEGER NOT NULL DEFAULT 0, str_max INTEGER NOT NULL DEFAULT 0, sta_max INTEGER NOT NULL DEFAULT 0,
			agi_max INTEGER NOT NULL DEFAULT 0, dex_max INTEGER NOT NULL DEFAULT 0, wis_max INTEGER NOT NULL DEFAULT 0, int_max INTEGER NOT NULL DEFAULT 0,
			cha_max INTEGER NOT NULL DEFAULT 0, hp_factor INTEGER NOT NULL DEFAULT 0, off_mod INTEGER NOT NULL DEFAULT 0, def_mod INTEGER NOT NULL DEFAULT 0,
			wpn1_dmg INTEGER NOT NULL DEFAULT 0, wpn1_type TEXT NOT NULL DEFAULT '', wpn2_dmg INTEGER NOT NULL DEFAULT 0, wpn2_type TEXT NOT NULL DEFAULT '',
			abilities TEXT NOT NULL DEFAULT '[]', keep_template INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'gm',
			size REAL NOT NULL DEFAULT 0)`,
		`INSERT INTO petstats (spellid, name) VALUES (591, 'Blade Swarm')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()
	if db, err = NewSQLite(path); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO petstats (spellid, name, melee_pct) VALUES (586, 'Iron Golem', 200), (566, 'Stone Golem', 0)`); err != nil {
		t.Fatal(err)
	}
	rows, bad, err := db.PetStatsRows()
	if err != nil || len(bad) != 0 {
		t.Fatalf("%v %v", err, bad)
	}
	for id, want := range map[int]int{591: 100, 586: 200, 566: 100} {
		if got := rows[id].MeleePct; got != want {
			t.Errorf("spell %d: melee_pct %d, want %d", id, got, want)
		}
	}
}
