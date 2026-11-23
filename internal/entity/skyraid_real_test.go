package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Live petition 5513 (the owner, 2026-11-23: the Plane of Sky's chains
// raid-style), on a server database with tools/sky_raid_20261123.py applied
// (EQOA_REAL_DB; skipped where it is not): the templates load with no
// problem, each chain's boss stands at 2000% of its level's hit points
// (Granavik over 300k), each isle creature at 300%, another world's flesh fiend
// at its own, and the Quintessence as she was.
func TestRealSkyChainsAtRaidStrength(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sky.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var applied int
	db.QueryRow(`SELECT count(*) FROM classtemplates WHERE name = 'sky raid boss'`).Scan(&applied)
	if applied == 0 {
		t.Skip("tools/sky_raid_20261123.py is not applied here")
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadBalance(db); err != nil { // the level's hit points, as at a start
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if p := w.ClassTemplateProblems(); len(p) != 0 {
		for _, s := range p {
			if strings.Contains(strings.ToLower(s), "sky raid") {
				t.Errorf("problem: %s", s)
			}
		}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	got := map[string][]*NPC{}
	for _, n := range w.NPCs {
		got[strings.ToLower(n.Name)] = append(got[strings.ToLower(n.Name)], n)
	}
	tpl := func(n *NPC) string {
		if t := w.templateForLocked(n); t != nil {
			return t.Name
		}
		return ""
	}
	for _, c := range []struct{ name, want string }{
		{"granavik", "sky raid boss"}, {"kaglanor", "sky raid boss"}, {"chimareon the tyrant", "sky raid boss"},
		{"necrologist", "sky raid boss caster"}, {"enraged aeyinar", "enraged aeyinar"},
		{"rancorous drake", "rancorous drake"}, {"dark servant", "sky raid creature"},
		{"ruthra's mage", "sky raid creature caster"}, {"quintessence of veeshan", "quintessence of veeshan"},
	} {
		ns := got[c.name]
		if len(ns) == 0 {
			t.Errorf("no %s", c.name)
			continue
		}
		for _, n := range ns {
			if tpl(n) != c.want {
				t.Errorf("%s (row %d): template %q, want %q", c.name, n.row(), tpl(n), c.want)
			}
		}
	}
	for _, n := range got["flesh fiend"] {
		sky := strings.HasPrefix(tpl(n), "sky raid")
		if n.World == 4 && tpl(n) != "sky raid creature" || n.World != 4 && sky {
			t.Errorf("flesh fiend of world %d at %d: template %q", n.World, n.Level, tpl(n))
		}
	}
	for _, n := range got["granavik"] {
		if n.MaxHP < 300000 {
			t.Errorf("Granavik at %d: %d hit points, want over 300,000", n.Level, n.MaxHP)
		}
		t.Logf("Granavik at %d: %d hit points", n.Level, n.MaxHP)
	}
	for _, n := range got["dark servant"][:1] {
		t.Logf("a dark servant at %d: %d hit points", n.Level, n.MaxHP)
	}
}
