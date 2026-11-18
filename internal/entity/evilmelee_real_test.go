package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/evil_melee_20261118.py's rows read as the server reads them
// (EQOA_REAL_DB names a database the tool was applied to; a copy is read):
// Boldunder's own template and Ahriaana's port of the one she fights read
// whole and with no problem, both are spawn-only (raised by their quests),
// the summoning's trigger has no problem and calls her on the main isle, and
// the summoning gem's line is a one-use self port to the Sky Portal's row.
func TestRealEvilMeleeBosses(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "em.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var applied int
	db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'evil_melee_1118_log'`).Scan(&applied)
	if applied == 0 {
		t.Skip("tools/evil_melee_20261118.py is not applied to this database")
	}
	tmpls, bad, err := db.ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		if b == "boldunder" || b == "ahriaana the ultimate" {
			t.Fatalf("%s is unreadable", b)
		}
	}
	b := tmpls["boldunder"]
	if b.HPPct != 5367 || b.MeleePct != 2058 || b.SpecialEvery != 80 || b.ResistPct != 75 || b.ResistKind != "FR,CR,LR,AR,PR,DR" {
		t.Fatalf("boldunder's template %+v", b)
	}
	a := tmpls["ahriaana the ultimate"]
	if len(a.Abilities) != 2 || a.Abilities[0].HealPct != 100 || a.Abilities[1].Kind != database.AbilityPortFoe ||
		a.Abilities[1].Every != 30 || a.Abilities[1].Chance != 50 || len(a.Abilities[1].To) != 0 {
		t.Fatalf("ahriaana's abilities %+v", a.Abilities)
	}
	uses, err := db.NPCTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if uses["boldunder"] != "boldunder" {
		t.Fatalf("npctemplate boldunder: %q", uses["boldunder"])
	}
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if l := strings.ToLower(p); strings.Contains(l, "boldunder") || strings.Contains(l, "ahriaana") {
			t.Errorf("problem: %s", p)
		}
	}
	if tm := w.templates.byName["ahriaana the ultimate"]; tm == nil || len(tm.moments) != 1 || tm.moments[0].Kind != database.AbilityPortFoe {
		t.Fatalf("ahriaana's moments: %+v", tm)
	}
	so, err := db.NPCSpawnOnly()
	if err != nil {
		t.Fatal(err)
	}
	if !so[database.NPCClassKey("Boldunder")] || !so[database.NPCClassKey("Ahriaana the Ultimate")] {
		t.Fatalf("spawn-only: %v", so)
	}
	var magi, inSky int
	for _, n := range w.NPCs {
		switch n.Name {
		case "Magi'Kot":
			magi++
			if n.World == 4 {
				inSky++
				if dx, dz := n.X-4834.8, n.Z-5524.7; dx*dx+dz*dz > 30*30 {
					t.Errorf("Magi'Kot on the Plane of Sky stands at (%.0f, %.0f), not by the landing", n.X, n.Z)
				}
			}
		case "Boldunder":
			if n.World != 2 || n.X != 11100 || n.Z != 5370 {
				t.Errorf("Boldunder's row at world %d (%.0f, %.0f), want the depression's middle", n.World, n.X, n.Z)
			}
		}
	}
	if magi != 2 || inSky != 1 {
		t.Fatalf("%d Magi'Kots, %d on the Plane of Sky: want the towers' and the isle's", magi, inSky)
	}

	trigs, err := db.GMTriggers()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tr := range trigs {
		if tr.Name != "Ahriaana the Ultimate" {
			continue
		}
		found = true
		if p := database.TriggerProblems(tr); len(p) != 0 {
			t.Errorf("trigger problems %q", p)
		}
		steps, err := database.ParseTriggerSteps(tr.Steps)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range steps {
			if s.Do == database.TriggerSpawn && (s.World == nil || *s.World != 4 || s.X != 4812 || s.Z != 5575) {
				t.Errorf("the summoning's spawn %+v, want the main isle", s)
			}
		}
	}
	if !found {
		t.Fatal("no trigger Ahriaana the Ultimate")
	}

	var line string
	if err := db.QueryRow("SELECT e.line FROM itemeffect e JOIN itempattern p USING (patternid) WHERE p.itemname = 'Magi''Kot''s summoning gem'").Scan(&line); err != nil {
		t.Fatal(err)
	}
	pe := NewPotionEffect(716990, "Magi'Kot's summoning gem", line, "", 0, "", 0, 0)
	// a teleport's words are the gate's (gates.go), the one thing the line reader leaves unread
	if place, ok := potionPlace(&pe.Def); len(pe.Def.Unread) != 1 || pe.Def.Unread[0] != "teleport to sky portal" || !ok || database.SpellGateKey(place) != "sky portal" || pe.Def.Unlimited {
		t.Fatalf("the gem's line %q: unread %q, place %q, unlimited %v", line, pe.Def.Unread, place, pe.Def.Unlimited)
	}
}
