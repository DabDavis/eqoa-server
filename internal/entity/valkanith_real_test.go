package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/valkanith_20261117.py's rows read as the server reads them (EQOA_REAL_DB
// names a database the tool was applied to): Lord Valkanith's template has no
// problem and its bloodtap is a pinned area life tap of a fixed 500, the rune's
// use reads whole, and the trigger has no problem.
func TestRealValkanith(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "v.db")
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
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(strings.ToLower(p), "valkanith") {
			t.Errorf("problem: %s", p)
		}
	}
	tm := w.templates.byName["lord valkanith"]
	if tm == nil {
		t.Fatal("no template lord valkanith")
	}
	if w.templates.use["lord valkanith"] != "lord valkanith" {
		t.Fatalf("npctemplate: %q", w.templates.use["lord valkanith"])
	}
	if len(tm.abilities) != 1 {
		t.Fatalf("abilities %d", len(tm.abilities))
	}
	a := tm.abilities[0]
	if a.pinned == nil || a.pinned.Area.Centre == AreaNone || a.Kind != database.AbilityLifetap || a.Damage != 500 {
		t.Fatalf("bloodtap %+v pinned %+v", a.TemplateAbility, a.pinned)
	}
	t.Logf("bloodtap %q area centre %v radius %v resist %v cast %v; template hp_pct %d", a.pinned.Name, a.pinned.Area.Centre,
		a.pinned.Area.Radius, a.pinned.resist, a.pinned.CastTime, tm.HPPct)

	var line string
	if err := db.QueryRow("SELECT e.line FROM itemeffect e JOIN itempattern p USING (patternid) WHERE p.itemname = 'Bone Wing Sentinel'").Scan(&line); err != nil {
		t.Fatal(err)
	}
	pe := NewPotionEffect(1, "Bone Wing Sentinel", line, "Thexian Bonewing", 0, "", 0, 0)
	if len(pe.Def.Unread) != 0 {
		t.Errorf("the rune's line leaves %q unread", pe.Def.Unread)
	}
	t.Logf("rune: %+v", pe.Def)

	trigs, err := db.GMTriggers()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tr := range trigs {
		if tr.Name == "Lord Valkanith" {
			found = true
			if p := database.TriggerProblems(tr); len(p) != 0 {
				t.Errorf("trigger problems %q", p)
			}
		}
	}
	if !found {
		t.Error("no trigger Lord Valkanith")
	}
}
