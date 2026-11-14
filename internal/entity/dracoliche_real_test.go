package entity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Dracoliche (the owner, 2026-11-14: "dracoliche is not in our database"), on a real server's database
// (EQOA_REAL_DB; a copy is read) with tools/dracoliche_20261114.py applied: one in world 0 at his guard spot in the
// Estate's west hall, level 70, a warrior, ~578,380 hit points by his template with no problem the world reports, a
// raid boss by the loot's rule, his add held out of the world and found by his summons' name, aggressive and seeing
// invisibility, back three days after his corpse goes. Skipped without a database, or one without him.
func TestRealDracoliche(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "draco.db")
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
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadRespawnRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		if strings.Contains(p, "dracoliche") || strings.Contains(p, "rotting disease") {
			t.Errorf("template problem: %s", p)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var draco, add *NPC
	for _, npc := range w.NPCs {
		switch npc.Name {
		case "Dracoliche":
			draco = npc
		case "Rotting Disease":
			add = npc
		}
	}
	if draco == nil {
		t.Skip("no Dracoliche in this database (tools/dracoliche_20261114.py not applied)")
	}
	w.applyBalanceLocked(draco)
	if draco.World != 0 || draco.X != 25550 || draco.Z != 3098 || draco.Level != 70 || draco.Class != 0 {
		t.Errorf("Dracoliche at world %d (%.1f, %.1f) level %d class %d", draco.World, draco.X, draco.Z, draco.Level, draco.Class)
	}
	if draco.tmpl == nil || draco.tmpl.Name != "dracoliche" {
		t.Fatalf("Dracoliche's template %v", draco.tmpl)
	}
	if d := draco.MaxHP - 578380; d < -500 || d > 500 {
		t.Errorf("Dracoliche's hit points %d, the wiki's 578,380", draco.MaxHP)
	}
	if !w.raidBossLocked(draco) {
		t.Error("Dracoliche is not a raid boss by the loot's rule")
	}
	if len(draco.tmpl.summons) != 2 || draco.tmpl.enrage == nil || draco.tmpl.enrage.Rampage != 50 {
		t.Errorf("Dracoliche's summons %d, enrage %+v", len(draco.tmpl.summons), draco.tmpl.enrage)
	}
	if w.npcSpawnOnlyLocked(draco) {
		t.Error("Dracoliche is held out of the world")
	}
	if add == nil || !w.npcSpawnOnlyLocked(add) || w.npcByNameLocked("Rotting Disease") == nil {
		t.Errorf("the add: %v, held out %v", add, add != nil && w.npcSpawnOnlyLocked(add))
	}
	if got := w.respawnTimeLocked(draco.ServerID); got != 72*time.Hour {
		t.Errorf("Dracoliche's respawn %v, want 72h", got)
	}
	r, ok := w.aggro.ruleFor("Dracoliche")
	if !ok || !r.Aggressive || !w.aggro.lookup("Dracoliche").seeInvis {
		t.Errorf("Dracoliche's aggro rule %+v %v, sees invisible %v", r, ok, w.aggro.lookup("Dracoliche").seeInvis)
	}
}
