package database

import (
	"database/sql"
	"reflect"
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
	// jib's five, the wisp and the roamer, and the pets' templates pettemplate gives the table
	// with it (pettemplates.go)
	if strings.Join(names, ",") != "tank,melee,wizard,necromancer,shadowknight,wisp,roamer" ||
		len(got) != len(SeedClassTemplates)+len(SeedPetClassTemplates) {
		t.Errorf("the seeds: %v (%d rows)", names, len(got))
	}
	for _, p := range SeedPetClassTemplates {
		if _, ok := got[p.Name]; !ok {
			t.Errorf("the pets' %q not in the table", p.Name)
		}
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
	// the wisp is jib's wizard but for its abilities' figures, 28% where his are 35% (the user's "28%")
	wz, ws := got["wizard"], got["wisp"]
	if len(ws.Abilities) != len(wz.Abilities) || ws.Class != wz.Class || ws.HPPct != wz.HPPct || ws.TakenPct != wz.TakenPct ||
		ws.MeleePct != wz.MeleePct || ws.Special != wz.Special || ws.SpecialEvery != wz.SpecialEvery {
		t.Errorf("the wisp %+v is not the wizard %+v", ws, wz)
	}
	for i := range ws.Abilities {
		a, b := ws.Abilities[i], wz.Abilities[i]
		if a.DamagePct != 28 || b.DamagePct != 35 {
			t.Errorf("ability %d: the wisp's %d%%, the wizard's %d%% (want 28 and 35)", i, a.DamagePct, b.DamagePct)
		}
		a.DamagePct = b.DamagePct
		if !reflect.DeepEqual(a, b) {
			t.Errorf("ability %d: the wisp's %+v is not the wizard's %+v but for its figure", i, ws.Abilities[i], b)
		}
	}
	if r := got["roamer"]; r.Class != 0 || r.HPPct != 100 || r.SpecialChance != 0 || r.SpecialDamage != 7.4 ||
		r.SpecialEvery != 30 || r.SpecialFirst != 15 || r.SpecialJitter != 5 || r.SpecialFrom != 10 || len(r.Abilities) != 0 {
		t.Errorf("the roamer: %+v", r)
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
	if _, ok := got["tank"]; ok || len(got) != len(SeedClassTemplates)+len(SeedPetClassTemplates)-1 {
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
	if err != nil || len(again) != 2 || !reflect.DeepEqual(again, list) {
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
	if m := got["melee"]; m.SpecialChance != 20 || m.SpecialEvery != 0 || m.SpecialFirst != 0 || m.SpecialJitter != 0 || m.SpecialFrom != 0 {
		t.Errorf("an older row: %+v (want its kick kept and no clock)", m)
	}
	for _, q := range []string{
		`INSERT INTO classtemplates (name, class, special_damage, special_every, special_first, special_jitter) VALUES ('roamer', 0, 7.4, 30, 15, 5)`,
		`INSERT INTO classtemplates (name, class, special_damage, special_every) VALUES ('toofast', 0, 7.4, 3)`,
		`INSERT INTO classtemplates (name, class, special_damage, special_every, special_jitter) VALUES ('wobbly', 0, 7.4, 30, 15)`,
		`INSERT INTO classtemplates (name, class, special_every) VALUES ('toothless', 0, 30)`,
		`INSERT INTO classtemplates (name, class, special_damage, special_every, special_from) VALUES ('ancient', 0, 7.4, 30, 101)`,
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
	for _, name := range []string{"toofast", "wobbly", "toothless", "ancient"} {
		if _, ok := got[name]; ok {
			t.Errorf("%q read, want left out", name)
		}
	}
	if len(bad) != 4 {
		t.Errorf("bad: %v, want the four", bad)
	}
}

// pettemplate reaches a database whose classtemplates was made before it:
// the pets' templates go in beside a GM's rows (a GM's row of a pet
// template's name kept as the GM left it) and pettemplate takes its seed
// rows, once; emptied by a GM, it stays empty at the next start, and a pet
// template a GM deleted stays deleted.
func TestPetTemplatesReachAnOlderDatabase(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	// as a database made before pettemplate: the table gone, a GM's row of a
	// pet template's name, the pets' other templates not there
	if _, err := db.Exec(`DROP TABLE pettemplate`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM classtemplates WHERE name LIKE 'pet %'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO classtemplates (name, class, hp_pct, note, source) VALUES ('pet earth', 0, 222, 'a GM''s', 'gm')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopen := func() *SQLiteDB {
		t.Helper()
		db, err := NewSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	db = reopen()
	got, bad, err := db.ClassTemplates()
	if err != nil || len(bad) != 0 {
		t.Fatalf("templates: %v, bad %v", err, bad)
	}
	for _, p := range SeedPetClassTemplates {
		if _, ok := got[p.Name]; !ok {
			t.Errorf("%q not given to the older table", p.Name)
		}
	}
	if got["pet earth"].HPPct != 222 {
		t.Errorf("the GM's pet earth: hp_pct %d, want its 222 kept", got["pet earth"].HPPct)
	}
	rows, _, err := db.PetTemplates()
	if err != nil || len(rows) != len(SeedPetTemplates) {
		t.Fatalf("pettemplate: %d rows, %v; want %d", len(rows), err, len(SeedPetTemplates))
	}
	if r := rows[PetTemplateKey{"howling death", true}]; r.Template != "pet necromancer weapon" || r.Swing != 7 || r.RoundMS != 5200 {
		t.Errorf("Howling Death's weapon version: %+v", r)
	}
	if _, err := db.Exec(`DELETE FROM pettemplate`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM classtemplates WHERE name = 'pet fire'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db = reopen()
	defer db.Close()
	rows, _, _ = db.PetTemplates()
	got, _, _ = db.ClassTemplates()
	if _, fire := got["pet fire"]; len(rows) != 0 || fire {
		t.Errorf("after a GM emptied pettemplate and deleted pet fire: %d rows, pet fire back %v", len(rows), fire)
	}
}

// A pettemplate row is held to a spell, a template, a swing class of the
// fifteen or -1, a CLIENT swing or -1, and a round of 1-20 s or 0; a pet's
// ability fields to what their kinds take.
func TestPetTemplateChecks(t *testing.T) {
	ok := PetTemplate{Spell: "storm of blades", Template: "pet animation", SwingClass: -1, Swing: -1}
	if err := CheckPetTemplate(ok); err != nil {
		t.Fatalf("a good row: %v", err)
	}
	for _, bad := range []PetTemplate{
		{Template: "pet animation", SwingClass: -1, Swing: -1},
		{Spell: "storm of blades", SwingClass: -1, Swing: -1},
		{Spell: "x", Template: "y", SwingClass: 15, Swing: -1},
		{Spell: "x", Template: "y", SwingClass: -1, Swing: 12},
		{Spell: "x", Template: "y", SwingClass: -1, Swing: 7, RoundMS: 500},
	} {
		if CheckPetTemplate(bad) == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
	for _, text := range []string{
		`[{"kind": "taunt", "every": 9, "hits": [[1, 84, 88]]}]`,
		`[{"kind": "buff", "stat": "ac", "spells_of": 7, "every": 20}]`,
		`[{"kind": "heal", "target": "owner", "below": 60, "every": 12}]`,
		`[{"kind": "heal", "heal_pct": 45, "every": 38}]`,
		`[{"kind": "nuke", "element": "fire", "spells_of": 10, "every": 12}]`,
	} {
		if _, err := ParseTemplateAbilities(text); err != nil {
			t.Errorf("%s: %v", text, err)
		}
	}
	for _, text := range []string{
		`[{"kind": "buff", "every": 20}]`,                                    // no stat
		`[{"kind": "buff", "stat": "luck", "every": 20}]`,                    // no such stat
		`[{"kind": "buff", "stat": "ac", "target": "self", "every": 20}]`,    // a buff on itself
		`[{"kind": "nuke", "target": "owner", "every": 20}]`,                 // a target on a nuke
		`[{"kind": "nuke", "element": "wind", "every": 20}]`,                 // no such element
		`[{"kind": "heal", "heal_pct": 120, "every": 20}]`,                   // past 100
		`[{"kind": "nuke", "heal_pct": 10, "every": 20}]`,                    // heal_pct on a nuke
		`[{"kind": "nuke", "hits": [[1, 84, 88]], "every": 20}]`,             // hits on a nuke
		`[{"kind": "taunt", "hits": [[60, 84, 88], [1, 1, 2]], "every": 9}]`, // levels falling
		`[{"kind": "taunt", "hits": [[1, 88, 84]], "every": 9}]`,             // least over most
		`[{"kind": "buff", "stat": "ac", "spells_of": 15, "every": 20}]`,     // no class 15
	} {
		if _, err := ParseTemplateAbilities(text); err == nil {
			t.Errorf("%s was taken", text)
		}
	}
}

// The mastery pets fight by templates of their own (the balance's
// pet fit of 2026-10-30): each summoning's pettemplate row names its own, and
// that template is its kind's but for its hit points and note, scaled down
// (a mastery pet a modest step over the book's pet of its level, no red won
// alone); the control: the book's pets of the same kinds keep their kind's.
func TestTheMagiciansMasteryPetsHaveTheirOwn(t *testing.T) {
	kinds := map[string]ClassTemplate{}
	for _, ct := range SeedPetClassTemplates {
		kinds[ct.Name] = ct
	}
	rows := map[string]string{}
	for _, p := range SeedPetTemplates {
		if !p.Weapon {
			rows[p.Spell] = p.Template
		}
	}
	for _, c := range []struct{ spell, own, kind string }{
		{"summon badi", "pet badi", "pet earth"}, {"summon efreet", "pet efreet", "pet fire"},
		{"summon djinn", "pet djinn", "pet air"}, {"summon badi elder", "pet badi elder", "pet earth"},
		{"summon efreeti lord", "pet efreeti lord", "pet fire"}, {"summon ghilan noble", "pet ghilan noble", "pet water"},
		{"gymon's animation", "pet gymon's animation", "pet animation"}, {"earthen golem", "pet earthen golem", "pet animation"},
		{"ferrous golem", "pet ferrous golem", "pet animation"},
	} {
		if rows[c.spell] != c.own {
			t.Errorf("%q fights by %q, want %q", c.spell, rows[c.spell], c.own)
		}
		own, kind := kinds[c.own], kinds[c.kind]
		if own.Name == "" || own.HPPct >= kind.HPPct || own.Class != kind.Class || own.TakenPct != kind.TakenPct ||
			own.MeleePct != kind.MeleePct || !reflect.DeepEqual(own.Abilities, kind.Abilities) {
			t.Errorf("%q: %+v, want %q's but for fewer hit points (%d%%)", c.own, own, c.kind, kind.HPPct)
		}
	}
	for spell, kind := range map[string]string{"earth elemental": "pet earth", "fire elemental": "pet fire",
		"air elemental": "pet air", "summon ghilan": "pet water", "stone golem": "pet animation", "iron golem": "pet animation"} {
		if rows[spell] != kind {
			t.Errorf("the control: %q fights by %q, want its kind's %q", spell, rows[spell], kind)
		}
	}
	if w := kinds["pet necromancer weapon"]; w.MeleePct != 100 {
		t.Errorf("the necromancer's weapon pet hits at %d%%, want 100", w.MeleePct)
	}
	if a := kinds["pet animation"]; a.HPPct != 100 || a.TakenPct != 100 || len(a.Abilities) != 5 {
		t.Errorf("the animation: %+v, want 100/100 and its taunt and four heals", a)
	}
}
