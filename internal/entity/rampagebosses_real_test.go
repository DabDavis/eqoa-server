package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/rampage_bosses_20261117.py's rows read as the server reads them (EQOA_REAL_DB names a
// database the tool was applied to): no template is unreadable, none of the rampage bosses' has a
// problem, and every template holding a rampage reads it as its area rampage.
func TestRealRampageBosses(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "r.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if strings.Contains(b, "rampage") {
			t.Errorf("unreadable: %s", b)
		}
	}
	w := NewWorld()
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "rampage") {
			t.Errorf("problem: %s", p)
		}
	}
	n := 0
	for name, row := range rows {
		for _, a := range row.Abilities {
			if a.Kind != database.AbilityRampage {
				continue
			}
			n++
			tm := w.templates.byName[name]
			if tm == nil || tm.areaRampage == nil {
				t.Errorf("%q: its rampage not read", name)
				continue
			}
			t.Logf("%q: radius %g chance %d every %g damage %d%%", name, tm.areaRampage.RadiusOr(), tm.areaRampage.Chance,
				tm.areaRampage.Every, tm.areaRampage.DamageOr())
		}
	}
	t.Logf("%d templates with a rampage", n)
}
