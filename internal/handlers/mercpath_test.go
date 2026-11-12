package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// saidLine reports a message that holds want.
func saidLine(msgs []string, want string) bool {
	return slices.ContainsFunc(msgs, func(m string) bool { return strings.Contains(m, want) })
}

func bookNames(b []entity.CastSpell) []string {
	var out []string
	for _, s := range b {
		out = append(out, s.Name)
	}
	return out
}

// A merc's book by its path (mercpath.go; the owner, 2026-11-11: "blood nec
// or power nec and they both use their path spells. same with all classes"):
// the path's line kept, the other paths' dropped, every spell no path names
// kept; a path not picked (-1) its class's default (a cleric's heals, else the
// first); a word picks a path by its name or a spell's, two paths' word none.
func TestMercPathBooks(t *testing.T) {
	book := func(names ...string) []entity.CastSpell {
		var b []entity.CastSpell
		for _, n := range names {
			b = append(b, entity.CastSpell{Name: n})
		}
		return b
	}
	nec := book("Lifetap", "Blood Squall", "Power Squall", "Blood Storm", "Power Storm")
	for _, c := range []struct {
		class, path int
		in          []entity.CastSpell
		want        []string
	}{
		{11, -1, nec, []string{"Lifetap", "Blood Squall", "Blood Storm"}},
		{11, 1, nec, []string{"Lifetap", "Power Squall", "Power Storm"}},
		{9, -1, book("Heal", "Poison Ward", "Bind Wounds", "Elemental Guard"), []string{"Heal", "Bind Wounds"}},
		{13, 1, book("Static Clash", "Mental Surge", "Ice Bolt"), []string{"Mental Surge", "Ice Bolt"}},
	} {
		if got := bookNames(mercPathBook(c.class, c.path, c.in)); !slices.Equal(got, c.want) {
			t.Errorf("%s path %d: %q, want %q", entity.ClassName(c.class), c.path, got, c.want)
		}
	}
	for _, c := range []struct {
		class int
		word  string
		want  int
		ok    bool
	}{{11, "blood", 0, true}, {11, "Power", 1, true}, {12, "boon", 1, true}, {12, "power", 1, true}, {12, "mind", 0, true},
		{9, "heal", 1, true}, {11, "squall", 0, false}, {11, "frog", 0, false}} {
		if k, ok := mercPathNamed(c.class, c.word); ok != c.ok || ok && k != c.want {
			t.Errorf("%s %q: %d %v, want %d %v", entity.ClassName(c.class), c.word, k, ok, c.want, c.ok)
		}
	}
}

// /merc path: a necromancer merc hired with no path picked takes the Blood
// path (its class's first), said at the hire; /merc path names the paths and
// changes nothing (the path is the hire's, 2026-11-12); a hire of the Power
// path keeps it in its row and its kit is the Power line's.
func TestMercPathCommand(t *testing.T) {
	r, m, db := newMercFixture(t)
	for _, s := range []struct {
		id   int
		name string
	}{{700323, "Blood Squall"}, {700341, "Power Squall"}} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, 30, 0, 0, 16880, 3, 100, 0, 0, 2, 10, 0, ?, '')`, s.id, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, 'Group, Increase Hitpoints by 300, 30min', 0, 'test')`, s.id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (11, ?, 'eqoa-data')`, s.id); err != nil {
			t.Fatal(err)
		}
	}
	m.hire(r.anna, r.pa, 11, 0, 0)
	got := r.msgs(r.anna)
	if !saidLine(got, " takes the path of Blood, for good.") {
		t.Errorf("the hire's lines: %q", got)
	}
	row, ok, _ := db.Merc(r.pa.CharServerID, 1)
	if !ok || row.Path != -1 {
		t.Fatalf("the hire's row: %v path %d", ok, row.Path)
	}
	kit, _ := m.kit(row, 30)
	if n := bookNames(kit.Spells); !slices.Contains(n, "Blood Squall") || slices.Contains(n, "Power Squall") {
		t.Errorf("the default path's book: %q", n)
	}
	r.drain()
	r.say(r.anna, "/merc path")
	if got := r.msgs(r.anna); !saidLine(got, "'s path: Blood, taken at its hire. Paths of a Necromancer: Blood, Power.") {
		t.Errorf("/merc path: %q", got)
	}
	// once hired, the path is the hire's for good (2026-11-12)
	r.say(r.anna, "/merc path power")
	if got := r.msgs(r.anna); !saidLine(got, " keeps to the path of Blood, taken at its hire.") {
		t.Errorf("/merc path power: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Path != -1 {
		t.Errorf("the path changed: %d", row.Path)
	}
	// a hire of the Power path at the board
	r.say(r.anna, "/merc dismiss")
	m.hireWith(r.anna, r.pa, 11, 0, 0, 1)
	row, _, _ = db.Merc(r.pa.CharServerID, 1)
	if row.Path != 1 {
		t.Fatalf("the path kept: %d", row.Path)
	}
	kit, _ = m.kit(row, 30)
	if n := bookNames(kit.Spells); slices.Contains(n, "Blood Squall") || !slices.Contains(n, "Power Squall") {
		t.Errorf("the Power path's book: %q", n)
	}
}
