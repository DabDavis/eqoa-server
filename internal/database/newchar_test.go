package database

import (
	"fmt"
	"testing"
)

func newCharDB(t *testing.T) (*SQLiteDB, *SQLiteCharacterStore) {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/newchar.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (10, 4, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Mending', ''), (11, 1, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Spark', ''),
		        (12, 1, 0, 0, 16880, 2, 15, 1, 2, 0, 5, -1, 'Going Home', ''), (13, 9, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Sword Art', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (7, 10, 'test'), (7, 11, 'test'), (7, 12, 'test'), (0, 13, 'test'), (0, 12, 'test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db, NewSQLiteCharacterStore(db)
}

type bookRow struct{ spell, order, on, where, lockout, show int }

func book(t *testing.T, db *SQLiteDB, sid uint32) []bookRow {
	t.Helper()
	rows, err := db.Query(`SELECT spellid, addedorder, onHotBar, whereonBar, unk1, showhide FROM spells WHERE serverid = ? ORDER BY addedorder`, sid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []bookRow
	for rows.Next() {
		var r bookRow
		if err := rows.Scan(&r.spell, &r.order, &r.on, &r.where, &r.lockout, &r.show); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// With the rules on: the rules' level whatever the record asked, and the
// class's spells in level then name order, off the bar, as a scribed spell
// goes in; another class's are not given.
func TestCreateNewWithTheRules(t *testing.T) {
	db, s := newCharDB(t)
	s.SetNewCharacterRules(NewCharacterRules{Level: 50, AllSpells: true})
	c := &CharacterRecord{CharName: "Tester", AccountID: 1, TClass: 7, Level: 3}
	n, err := s.CreateNew(c)
	if err != nil || n != 3 {
		t.Fatalf("spells %d, %v", n, err)
	}
	if got, _ := s.GetByServerID(c.ServerID); got.Level != 50 {
		t.Errorf("level %d", got.Level)
	}
	want := []bookRow{{12, 0, 0, -1, 0, -1}, {11, 1, 0, -1, 0, -1}, {10, 2, 0, -1, 0, -1}}
	got := book(t, db, c.ServerID)
	if len(got) != len(want) {
		t.Fatalf("book %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("book[%d] %+v, want %+v", i, got[i], want[i])
		}
	}
	// granting again adds only what is missing, after what is there
	if _, err := db.Exec(`DELETE FROM spells WHERE serverid = ? AND spellid = 11`, c.ServerID); err != nil {
		t.Fatal(err)
	}
	if n, err := s.GrantClassSpells(c.ServerID, 7); err != nil || n != 1 {
		t.Fatalf("regrant %d, %v", n, err)
	}
	if got := book(t, db, c.ServerID); len(got) != 3 || got[2] != (bookRow{11, 3, 0, -1, 0, -1}) {
		t.Errorf("after the regrant %+v", got)
	}
}

// The control: with no rules, level 1 whatever the record asked (a client's
// creation message says a level), and no spells.
func TestCreateNewWithoutRules(t *testing.T) {
	db, s := newCharDB(t)
	c := &CharacterRecord{CharName: "Plain", AccountID: 1, TClass: 7, Level: 60}
	if n, err := s.CreateNew(c); err != nil || n != 0 {
		t.Fatalf("spells %d, %v", n, err)
	}
	if got, _ := s.GetByServerID(c.ServerID); got.Level != 1 {
		t.Errorf("level %d, want 1", got.Level)
	}
	if got := book(t, db, c.ServerID); len(got) != 0 {
		t.Errorf("book %+v", got)
	}
	s.SetNewCharacterRules(NewCharacterRules{Level: 99})
	c2 := &CharacterRecord{CharName: "Capped", AccountID: 1, TClass: 0}
	if _, err := s.CreateNew(c2); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetByServerID(c2.ServerID); got.Level != 60 {
		t.Errorf("level %d, want at most 60", got.Level)
	}
}

// homeDB adds start spots: a human of the east's warrior and SK, a human of
// the west's warrior, a gnome's enchanter, and spellpattern rows for the
// starting abilities of a warrior and an enchanter.
func homeDB(t *testing.T) (*SQLiteDB, *SQLiteCharacterStore) {
	t.Helper()
	db, s := newCharDB(t)
	for _, q := range []string{
		`INSERT INTO defaultcharacter (tclass, race, humType, level, tunar, bankTunar, unusedTP, totalTP, x, y, z, facing, world)
		 VALUES (0, 0, 1, 1, 0, 0, 0, 0, 25486.5, 61.4, 15547.0, 1.5, 0),
		        (3, 0, 1, 1, 0, 0, 0, 0, 25478.1, 53.5, 15607.3, 0, 0),
		        (0, 0, 2, 1, 0, 0, 0, 0, 4243.7, 58.1, 17169.1, 0, 0),
		        (12, 3, 0, 1, 0, 0, 0, 0, 23757.3, 32.6, 6123.1, 2.25, 0),
		        (0, 9, 0, 1, 0, 0, 0, 0, 9377.7, 53.2, 7385.8, 0, 1)`,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (1517, 1, 0, 0, 0, 0, 8, 0, 0, 1, 30, 63, 'Quick Strike', ''), (1623, 1, 0, 0, 0, 60, 25, 0, 0, 0, 5, -1, 'Return Home', ''),
		        (231, 1, 0, 0, 0, 2, 10, 0, 0, 1, 6, -1, 'Lull', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db, s
}

// A new character starts on its race's, class's and city's row, bound
// there by its city's name, with its class's first ability and Return Home;
// a froglok as a human of the east; a class with no row on its race's row;
// a race with none at the origin, unbound (the login spawn takes it).
func TestCreateNewStartsInTheHomeCity(t *testing.T) {
	db, s := homeDB(t)
	cases := []struct {
		name             string
		race, class, hum int
		x, z, face       float32
		world            int
		city             string
		book             []int
	}{
		{"Eastman", 0, 0, 1, 25486.5, 15547.0, 1.5, 0, "Freeport", []int{1517, 1623}},
		{"Westman", 0, 0, 2, 4243.7, 17169.1, 0, 0, "Qeynos", []int{1517, 1623}},
		{"Nocity", 0, 0, 0, 25486.5, 15547.0, 1.5, 0, "Freeport", []int{1517, 1623}},   // the east's
		{"Ribbit", raceFroglok, 3, 0, 25478.1, 15607.3, 0, 0, "Freeport", []int{1623}}, // SK: Chilling Aura absent here
		{"Tinker", 3, 12, 0, 23757.3, 6123.1, 2.25, 0, "Klik'Anon", []int{231, 1623}},
		{"Brute", 9, 0, 0, 9377.7, 7385.8, 0, 1, "Oggok", []int{1517, 1623}},
		{"Oddgnome", 3, 0, 0, 23757.3, 6123.1, 2.25, 0, "Klik'Anon", []int{1517, 1623}}, // no row: the race's
	}
	for _, c := range cases {
		r := &CharacterRecord{CharName: c.name, AccountID: 1, Race: c.race, TClass: c.class, HumType: c.hum}
		n, err := s.CreateNew(r)
		if err != nil || n != len(c.book) {
			t.Fatalf("%s: spells %d, %v", c.name, n, err)
		}
		got, _ := s.GetByServerID(r.ServerID)
		if got.X != c.x || got.Z != c.z || got.World != c.world || got.Facing != c.face {
			t.Errorf("%s: at (%.1f, %.1f) world %d facing %.2f, want (%.1f, %.1f) world %d facing %.2f",
				c.name, got.X, got.Z, got.World, got.Facing, c.x, c.z, c.world, c.face)
		}
		b, ok, err := s.LoadBind(r.ServerID)
		if !ok || err != nil || b.X != c.x || b.Z != c.z || b.World != c.world || b.Spiritmaster != c.city {
			t.Errorf("%s: bind %+v %v %v, want at (%.1f, %.1f) by %q", c.name, b, ok, err, c.x, c.z, c.city)
		}
		var ids []int
		for _, row := range book(t, db, r.ServerID) {
			ids = append(ids, row.spell)
		}
		if fmt.Sprint(ids) != fmt.Sprint(c.book) {
			t.Errorf("%s: book %v, want %v", c.name, ids, c.book)
		}
	}
	// a race with no row stays at the origin and unbound
	r := &CharacterRecord{CharName: "Nowhere", AccountID: 1, Race: 6, TClass: 0}
	if _, err := s.CreateNew(r); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetByServerID(r.ServerID); got.X != 0 || got.Z != 0 {
		t.Errorf("a race with no row moved to (%.1f, %.1f)", got.X, got.Z)
	}
	if _, ok, _ := s.LoadBind(r.ServerID); ok {
		t.Errorf("a race with no row was bound")
	}
	// a place already given is kept and not bound
	r = &CharacterRecord{CharName: "Placed", AccountID: 1, Race: 0, TClass: 0, HumType: 1, X: 10, Y: 20, Z: 30}
	if _, err := s.CreateNew(r); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetByServerID(r.ServerID); got.X != 10 || got.Z != 30 {
		t.Errorf("a given place moved to (%.1f, %.1f)", got.X, got.Z)
	}
}

// Every class has a starting ability and Return Home; the measured ones are
// Sandstorm's ids.
func TestStartingAbilities(t *testing.T) {
	for class := 0; class < 15; class++ {
		a := StartingAbilities(class)
		if len(a) != 2 || a[1] != returnHome || a[0] == 0 {
			t.Errorf("class %d: %v", class, a)
		}
	}
	measured := map[int]int{0: 1517, 1: 1096, 2: 1006, 3: 1420, 4: 785, 6: 1213, 7: 457, 9: 457, 12: 231, 14: 4}
	for class, id := range measured {
		if StartingAbilities(class)[0] != id {
			t.Errorf("class %d starts with %d, Sandstorm's %d", class, StartingAbilities(class)[0], id)
		}
	}
	if got := StartingAbilities(99); len(got) != 1 || got[0] != returnHome {
		t.Errorf("an unknown class: %v", got)
	}
}

// A were form or a were's ability in a class's list is never given by a
// class grant (#template, start_all_spells): the lion forms a necromancer's
// list held (F3 20261025-220040). The control: the class's other spells are.
func TestGrantClassSpellsSkipsWereAbilities(t *testing.T) {
	db, s := newCharDB(t)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (1816, 30, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Lionwere', 'Assume the form of the lionwere.'),
		        (1810, 40, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Feral Lion Form', 'CM: Illusion'),
		        (6116, 50, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Sabretooth Lion Form', 'This spell grants the power of the beast without changing your form.'),
		        (1200, 40, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1, 'Distracting Squeeks', 'Wererat Melee Ability - Debuff AC'),
		        (496, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Glamour: Human', 'CM: Illusion and faction change.'),
		        (565, 30, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Illusion: Troll', 'Illusion only'),
		        (875, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, -1, 'Disease Cloud', 'A cloud of infection harms your enemy.')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (11, 1816, 'dump'), (11, 1810, 'dump'), (11, 6116, 'dump'),
		        (11, 1200, 'dump'), (11, 496, 'dump'), (11, 565, 'dump'), (11, 875, 'dump')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c := &CharacterRecord{CharName: "Necro", AccountID: 1, TClass: 11}
	if err := s.Create(c); err != nil {
		t.Fatal(err)
	}
	n, err := s.GrantClassSpells(c.ServerID, 11)
	if err != nil || n != 3 {
		t.Fatalf("given %d, %v", n, err)
	}
	var ids []int
	for _, row := range book(t, db, c.ServerID) {
		ids = append(ids, row.spell)
	}
	if fmt.Sprint(ids) != "[875 496 565]" {
		t.Errorf("book %v, want Disease Cloud, Glamour: Human, Illusion: Troll", ids)
	}
}

func TestIsWereAbility(t *testing.T) {
	for _, w := range [][2]string{
		{"Ratwere", "Assume the form of the ratwere."}, {"Werewolf Form", "Assume the form of the werewolf."},
		{"Mouse Form", "Assume the form of a mouse."}, {"Feral Rat Form", "CM: Illusion"},
		{"Feral Lion Form", "This spell grants the power of the beast without changing your form."},
		{"Defense of the Swarm", "Wererat Tank Buff - Buff AC, STR"}, {"Bearwere", ""}, {"Gatorwere", ""},
	} {
		if !IsWereAbility(w[0], w[1]) {
			t.Errorf("%q (%q) is a were's", w[0], w[1])
		}
	}
	for _, w := range [][2]string{
		{"True Form", "Returns Caster to Natural Self."}, {"Glamour: Elf", "CM: Illusion and faction change."},
		{"Illusion: Dark Elf", "Illusion only"}, {"Remove Illusion", "This ability removes the current illusion on you."},
		{"Lupine Guise", ""}, {"Spirit of the Wolf", "Increases run speed."}, {"Hardening Agent", "were you to"},
	} {
		if IsWereAbility(w[0], w[1]) {
			t.Errorf("%q (%q) is not a were's", w[0], w[1])
		}
	}
}
