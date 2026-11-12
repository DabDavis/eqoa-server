package entity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/quil_20261112.py's template reads as the server reads it (EQOA_REAL_DB
// names a database the tool was applied to): "epic quileyne" not unreadable,
// her three 2,000s each every 120 s, 40 s apart (the guide's critical, nuke and
// taunt-like spell), the Drain as it was, and no template problem.
func TestRealQuilEyneTemplate(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "quil.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tmpls, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if b == "epic quileyne" {
			t.Fatalf("epic quileyne is unreadable: %q", bad)
		}
	}
	q, ok := tmpls["epic quileyne"]
	if !ok {
		t.Fatal("no template epic quileyne")
	}
	if q.Special != "Critical Hit" || q.SpecialEvery != 120 || q.SpecialFirst != 6 || q.HPPct != 9660 || q.MeleePct != 1497 {
		t.Fatalf("the template %+v", q)
	}
	type want struct {
		name         string
		damage       int
		every, first float64
	}
	wants := []want{{"Blood Strike", 2000, 120, 46}, {"Vampiric Taunt", 2000, 120, 86}, {"Drain", 500, 10.5, 8.8}}
	if len(q.Abilities) != len(wants) {
		t.Fatalf("abilities %+v", q.Abilities)
	}
	for i, w := range wants {
		a := q.Abilities[i]
		if a.Kind != database.AbilityNuke || a.Name != w.name || a.Damage != w.damage || a.Every != w.every || a.First != w.first {
			t.Errorf("ability %d: %+v, want %+v", i, a, w)
		}
	}
	uses, err := db.NPCTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if uses["quil'eyne"] != "epic quileyne" {
		t.Fatalf("npctemplate quil'eyne: %q", uses["quil'eyne"])
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if p := w.ClassTemplateProblems(); len(p) != 0 {
		t.Errorf("problems: %q", p)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, npc := range w.NPCs {
		if npc.Name == "Quil'Eyne" {
			if npc.TemplateName() != "epic quileyne" || npc.Level != 65 {
				t.Errorf("Quil'Eyne fights by %q at level %d", npc.TemplateName(), npc.Level)
			}
			return
		}
	}
	t.Error("no Quil'Eyne loaded")
}
