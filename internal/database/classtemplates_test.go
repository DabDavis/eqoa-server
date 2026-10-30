package database

import (
	"database/sql"

	"strings"
	"testing"
)

// classtemplates is made with jib's five examples and npctemplate empty: no
// NPC fights by a template until a GM gives one. A seed a GM deleted stays
// deleted at the next start (the table is seeded only when it is made).
func TestClassTemplatesAreSeededAndNoneApplied(t *testing.T) {
	path := t.TempDir() + "/templates.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	got, bad, err := db.ClassTemplates()
	if err != nil || len(bad) != 0 {
		t.Fatalf("templates: %v, bad %v", err, bad)
	}
	var names []string
	for _, s := range SeedClassTemplates {
		names = append(names, s.Name)
		if _, ok := got[s.Name]; !ok {
			t.Errorf("seed %q not in the table", s.Name)
		}
	}
	if strings.Join(names, ",") != "tank,melee,wizard,necromancer,shadowknight" || len(got) != 5 {
		t.Errorf("the seeds: %v (%d rows)", names, len(got))
	}
	w := got["wizard"]
	if w.Class != 13 || len(w.Abilities) != 2 || w.Abilities[0].Kind != AbilityAoE || w.Abilities[1].Kind != AbilityNuke ||
		!w.Abilities[1].Chain || w.Abilities[0].Every != 15 || w.Abilities[0].MaxRecast != 10 {
		t.Errorf("the wizard as read back: %+v", w)
	}
	if n := got["necromancer"]; n.Class != 11 || len(n.Abilities) != 1 || n.Abilities[0].Kind != AbilityLifetap || n.Abilities[0].Every != 5 {
		t.Errorf("the necromancer: %+v", n)
	}
	if sk := got["shadowknight"]; sk.Class != 3 || sk.HPPct <= 100 || sk.TakenPct >= 100 || sk.Abilities[0].Every != 15 {
		t.Errorf("the shadow knight: %+v", sk)
	}
	if m := got["melee"]; m.MeleePct <= 100 || m.Special != "Kick" || m.SpecialChance <= 0 || m.SpecialDamage <= 0 {
		t.Errorf("the melee: %+v", m)
	}
	if tk := got["tank"]; tk.HPPct <= 100 || tk.TakenPct >= 100 || len(tk.Abilities) != 0 {
		t.Errorf("the tank: %+v", tk)
	}
	var source string
	if err := db.QueryRow(`SELECT source FROM classtemplates WHERE name = 'wizard'`).Scan(&source); err != nil || source != "seed" {
		t.Errorf("the seed's source %q, %v", source, err)
	}
	uses, err := db.NPCTemplates()
	if err != nil || len(uses) != 0 {
		t.Errorf("npctemplate: %v, %v; want empty: none applied", uses, err)
	}

	// a GM deletes one: it stays deleted
	if _, err := db.Exec(`DELETE FROM classtemplates WHERE name = 'tank'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, _, _ = db.ClassTemplates()
	if _, ok := got["tank"]; ok || len(got) != 4 {
		t.Errorf("after a GM's delete and a restart: %d templates, tank back %v", len(got), ok)
	}

	// an assignment is keyed as npcclass keys a name, its template as a template
	if _, err := db.Exec(`INSERT INTO npctemplate (name, template) VALUES ('Gnoll Shaman$ ', ' Wizard')`); err != nil {
		t.Fatal(err)
	}
	if uses, _ := db.NPCTemplates(); uses["gnoll shaman"] != "wizard" || len(uses) != 1 {
		t.Errorf("the assignment: %v", uses)
	}
}

// A row the game cannot fight by is left out and named; the rest are read.
// The control: the same row put right is read.
func TestABadClassTemplateIsLeftOut(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/bad.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO classtemplates (name, class, abilities) VALUES ('typo', 13, '[{"kind": "nuke", "evry": 10}]')`,
		`INSERT INTO classtemplates (name, class, abilities) VALUES ('chainfirst', 13, '[{"kind": "nuke", "chain": true}]')`,
		`INSERT INTO classtemplates (name, class, hp_pct) VALUES ('frail', 0, 0)`,
		`INSERT INTO classtemplates (name, class) VALUES ('noclass', 15)`,
		`INSERT INTO classtemplates (name, class, special, special_chance) VALUES ('nokick', 0, 'Kick', 20)`,
		`INSERT INTO classtemplates (name, class, abilities) VALUES ('fine', 13, '[{"kind": "Nuke", "every": 10, "line": " bolt "}]')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"typo", "chainfirst", "frail", "noclass", "nokick"} {
		if _, ok := got[name]; ok {
			t.Errorf("%q read, want left out", name)
		}
	}
	if len(bad) != 5 {
		t.Errorf("bad: %v, want the five named", bad)
	}
	if f, ok := got["fine"]; !ok || f.Abilities[0].Kind != AbilityNuke || f.Abilities[0].Line != "bolt" {
		t.Errorf("the control: %+v %v", f, ok)
	}
}

// The abilities' JSON: a misspelt field is refused (it would be its default
// silently), a chain needs something before it, a clock is 2 s to an hour; the
// defaults are what they say. Written back, a list reads the same.
func TestTemplateAbilities(t *testing.T) {
	for _, bad := range []string{
		`{"kind": "nuke"}`,
		`[{"kind": "fireball", "every": 10}]`,
		`[{"kind": "nuke", "every": 1}]`,
		`[{"kind": "nuke", "every": 10, "chance": 101}]`,
		`[{"kind": "nuke", "every": 10}] junk`,
		`[{"kind": "nuke", "every": 10, "damage": 50}]`,
	} {
		if _, err := ParseTemplateAbilities(bad); err == nil {
			t.Errorf("%s read, want refused", bad)
		}
	}
	list, err := ParseTemplateAbilities(`[{"kind": "aoe", "every": 15, "first": 2}, {"kind": "nuke", "chain": true, "damage_pct": 40}]`)
	if err != nil || len(list) != 2 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].ChanceOr() != 100 || list[0].DamageOr() != 100 || list[1].DamageOr() != 40 || list[0].BelowOr() != 50 {
		t.Errorf("defaults: %+v", list)
	}
	again, err := ParseTemplateAbilities(TemplateAbilitiesJSON(list))
	if err != nil || len(again) != 2 || again[1] != list[1] || again[0] != list[0] {
		t.Errorf("written back: %v %v", again, err)
	}
	if none, err := ParseTemplateAbilities(" "); err != nil || none != nil || TemplateAbilitiesJSON(nil) != "[]" {
		t.Errorf("none: %v %v", none, err)
	}
	for _, s := range SeedClassTemplates {
		if err := CheckClassTemplate(s); err != nil {
			t.Errorf("seed %q: %v", s.Name, err)
		}
	}
}

// A template's spells: its class's by level as classspells has them (a
// mastery's left out), with their lines, visuals and spellarea rows; a spell
// pinned by name is the record with a line first. The range's bits are the
// wire's.
func TestTemplateSpells(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/spells.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
			VALUES (1, 20, 0, 0, 16880, 2, 50, 0, 7, 1, 3, 0, 'Energy Storm', ''),
			       (2, 10, 0, 0, 16880, 2, 20, 0, 8, 1, 3, 0, 'Static Shock', ''),
			       (3, 15, 0, 0, 16880, 2, 20, 0, 9, 1, 3, 0, 'Fury', ''),
			       (4, 5, 0, 0, 16880, 2, 20, 0, 9, 1, 3, 0, 'Kick', ''),
			       (5, 3, 0, 0, 16880, 0, 23, 0, 9, 1, 6, 0, 'Kick', '')`,
		`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES
			(1, 'Target, Decrease Hitpoints by 210 aoe Decrease Hitpoints by 90', 1234, 't'),
			(2, 'Target, Decrease Hitpoints by 104', 0, 't'),
			(5, 'Target, Decrease Hitpoints by 16', 170421806, 't')`,
		`INSERT INTO spellarea (spellid, centre, radius, maxtargets) VALUES (1, 1, 20, 4)`,
		`INSERT INTO classspells (class, spellid, source) VALUES (13, 1, 'dump'), (13, 2, 'dump'), (13, 3, 'eqoa-data cm')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	spells, err := db.TemplateSpells(13)
	if err != nil || len(spells) != 2 {
		t.Fatalf("the wizard's: %v %v", spells, err)
	}
	if spells[0].Pattern.SpellName != "Static Shock" || spells[1].Pattern.SpellName != "Energy Storm" {
		t.Errorf("by level: %s, %s", spells[0].Pattern.SpellName, spells[1].Pattern.SpellName)
	}
	if s := spells[1]; s.FX != 1234 || !s.HasArea || s.Area.Radius != 20 || s.Area.MaxTargets != 4 || s.Pattern.CastTime != 2 {
		t.Errorf("Energy Storm: %+v", s)
	}
	if spells[0].HasArea {
		t.Errorf("Static Shock has no spellarea row")
	}
	k, ok, err := db.TemplateSpellNamed(" kick ")
	if err != nil || !ok || k.Pattern.SpellID != 5 || k.FX != 170421806 {
		t.Errorf("Kick: %+v %v %v, want the record with a line (5)", k, ok, err)
	}
	if _, ok, err := db.TemplateSpellNamed("Nothing Such"); ok || err != nil {
		t.Errorf("a spell of no record: %v %v", ok, err)
	}
	if b := k.Pattern.RangeBits(); b != 16880<<16 {
		t.Errorf("range bits %#x", b)
	}
}

// The timed special's columns: a classtemplates made before them gets each at
// 0 (no template has a clock until a GM gives it one), a row's clock reads
// back, and the checks refuse a clock under 5 s, a jitter of half the clock
// or more, and a clock with no damage; the control: a roamer's 30 s, 15 s in,
// 5 s either way, reads as written.
func TestATimedSpecialsColumns(t *testing.T) {
	path := t.TempDir() + "/old.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE classtemplates (name TEXT PRIMARY KEY, class INTEGER NOT NULL DEFAULT -1,
		hp_pct INTEGER NOT NULL DEFAULT 100, taken_pct INTEGER NOT NULL DEFAULT 100, melee_pct INTEGER NOT NULL DEFAULT 100,
		special TEXT NOT NULL DEFAULT '', special_chance INTEGER NOT NULL DEFAULT 0, special_damage REAL NOT NULL DEFAULT 0,
		special_fx INTEGER NOT NULL DEFAULT 0, abilities TEXT NOT NULL DEFAULT '[]', note TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm');
		INSERT INTO classtemplates (name, class, special, special_chance, special_damage) VALUES ('melee', 4, 'Kick', 20, 7.4);`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, bad, err := db.ClassTemplates()
	if err != nil || len(bad) != 0 {
		t.Fatalf("%v %v", err, bad)
	}
	if m := got["melee"]; m.SpecialChance != 20 || m.SpecialEvery != 0 || m.SpecialFirst != 0 || m.SpecialJitter != 0 {
		t.Errorf("an older row: %+v (want its kick kept and no clock)", m)
	}
	for _, q := range []string{
		`INSERT INTO classtemplates (name, class, special_damage, special_every, special_first, special_jitter) VALUES ('roamer', 0, 7.4, 30, 15, 5)`,
		`INSERT INTO classtemplates (name, class, special_damage, special_every) VALUES ('toofast', 0, 7.4, 3)`,
		`INSERT INTO classtemplates (name, class, special_damage, special_every, special_jitter) VALUES ('wobbly', 0, 7.4, 30, 15)`,
		`INSERT INTO classtemplates (name, class, special_every) VALUES ('toothless', 0, 30)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	got, bad, err = db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if r := got["roamer"]; r.SpecialEvery != 30 || r.SpecialFirst != 15 || r.SpecialJitter != 5 || r.SpecialDamage != 7.4 {
		t.Errorf("the roamer: %+v", r)
	}
	for _, name := range []string{"toofast", "wobbly", "toothless"} {
		if _, ok := got[name]; ok {
			t.Errorf("%q read, want left out", name)
		}
	}
	if len(bad) != 3 {
		t.Errorf("bad: %v, want the three", bad)
	}
}
