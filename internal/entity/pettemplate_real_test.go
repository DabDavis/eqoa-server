package entity

import (
	"fmt"
	"strings"
	"testing"
)

// The pets' templates over a real server's database (EQOA_REAL_DB, copied):
// every spell-casting ability of a seed pet template has a spell of its kind
// at the levels its pets are summoned at, and what each picks there is
// printed (-v), the list the templates' spells are chosen by.
func TestRealPetTemplatePicks(t *testing.T) {
	db := simRealDB(t)
	w := NewWorld()
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	levels := map[string][]int{
		"pet water":     {4, 20, 39, 47, 55},
		"pet earth":     {1, 16, 34, 49, 55},
		"pet fire":      {8, 24, 44, 49, 55},
		"pet air":       {12, 29, 45, 49},
		"pet animation": {1, 8, 16, 20, 24, 34, 44, 49, 55, 60},
	}
	for _, name := range []string{"pet water", "pet earth", "pet fire", "pet air", "pet animation"} {
		ct := w.templates.byName[name]
		if ct == nil {
			t.Fatalf("no %q template", name)
		}
		for i := range ct.abilities {
			a := &ct.abilities[i]
			what := a.Kind
			for _, f := range []string{a.Target, a.Stat, a.Element, a.Line} {
				if f != "" {
					what += " " + f
				}
			}
			if a.HealPct > 0 {
				what += fmt.Sprintf(" %d%% of its most", a.HealPct)
			}
			if a.Kind == "taunt" || a.HealPct > 0 && a.Spell == "" {
				t.Logf("%s: %s: no spell (its own figure)", name, what)
				continue
			}
			class := ct.Class
			if a.SpellsOf != nil {
				class = *a.SpellsOf
			}
			var picks []string
			for _, lvl := range levels[name] {
				s := a.spellFor(class, lvl)
				if s == nil {
					picks = append(picks, fmt.Sprintf("%d: none", lvl))
					if lvl >= 20 {
						t.Errorf("%s: %s: no spell of class %d at %d", name, what, class, lvl)
					}
					continue
				}
				picks = append(picks, fmt.Sprintf("%d: %s (%d)", lvl, s.Name, s.Level))
			}
			t.Logf("%s: %s (class %d): %s", name, what, class, strings.Join(picks, ", "))
		}
	}
}
