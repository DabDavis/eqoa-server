package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// jib's five templates over a real server's spells (EQOA_REAL_DB names its
// database; a copy is read): the database made at a start with this code has
// the five and gives none to any NPC, every ability finds a spell of its
// kind from the level its class first has one, and what each picks is the
// level's: the wizard's area spell never Phoenix Glory (its 15 s cast) and
// its nuke never a 180 s nova, the necromancer's tap the line's rank of its
// level, the shadow knight's its strike of the level, never the 300 s Blood
// line (Bloodwasp ... Bloodwraith). Skipped without a
// database: none is in this repository.
func TestRealClassTemplatesPickTheirLevelsSpells(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "templates.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	n, err := w.LoadClassTemplates(db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d NPCs fight by a template on a database no GM gave one", n)
	}
	for _, l := range w.ClassTemplateSummary() {
		t.Log(l)
	}
	if p := w.ClassTemplateProblems(); len(p) != 0 {
		t.Errorf("problems: %q", p)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	picks := map[string]map[int]string{}
	for _, name := range []string{"wizard", "necromancer", "shadowknight"} {
		tm := w.templates.byName[name]
		if tm == nil {
			t.Fatalf("no %s template", name)
		}
		picks[name] = map[int]string{}
		for i := range tm.abilities {
			a := &tm.abilities[i]
			var line []string
			for _, lvl := range []int{10, 15, 20, 25, 30, 35, 40, 45, 49, 50, 55, 60} {
				s := a.spellFor(tm.Class, lvl)
				what := "-"
				if s != nil {
					what = fmt.Sprintf("%s (%d, cast %v, recast %v)", s.Name, s.Level, s.CastTime, s.Recast)
					if lvl >= 20 && a.MaxCast > 0 && s.CastTime.Seconds() > a.MaxCast || a.MaxRecast > 0 && s.Recast.Seconds() > a.MaxRecast {
						t.Errorf("%s ability %d at %d: %s, past its bounds", name, i+1, lvl, what)
					}
				} else if lvl >= 20 {
					t.Errorf("%s ability %d (%s) finds nothing at level %d", name, i+1, a.Kind, lvl)
				}
				line = append(line, fmt.Sprintf("%d: %s", lvl, what))
				if s != nil {
					picks[name][lvl*10+i] = s.Name
				}
			}
			t.Logf("%s ability %d (%s): %s", name, i+1, a.Kind, strings.Join(line, "; "))
		}
	}
	for lvl := 10; lvl <= 60; lvl += 5 {
		for _, bad := range []string{"Phoenix Glory", "Nova", "Explode", "Detonate", "Annihilat", "Icy", "Static Nova"} {
			for i := 0; i < 2; i++ {
				if s := picks["wizard"][lvl*10+i]; strings.Contains(s, bad) {
					t.Errorf("the wizard at %d casts %s", lvl, s)
				}
			}
		}
		for _, blood := range []string{"Bloodwasp", "Bloodraven", "Bloodleech", "Bloodghoul", "Bloodwraith"} {
			if s := picks["shadowknight"][lvl*10]; s == blood {
				t.Errorf("the shadow knight at %d taps with %s (300 s recast)", lvl, s)
			}
		}
	}
	if s := picks["necromancer"][490]; s != "Drain Soul" {
		t.Errorf("the necromancer at 49 taps with %q, want Drain Soul (48)", s)
	}
	if s := picks["shadowknight"][490]; s != "Doomstrike" {
		t.Errorf("the shadow knight at 49 taps with %q, want Doomstrike", s)
	}
}
