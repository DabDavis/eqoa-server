package entity

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Virulent Blades (the owner, 2026-11-14: "lets add virulent blades to the database IoD mob"), on a real server's
// database (EQOA_REAL_DB; a copy is read) with tools/virulent_blade_20261114.py applied: six in world 0 on the Isle of
// Dread, level 53, as the island's wicker men are (their template and so their hit points, their loot region), each
// wandering on land, aggressive and seeing invisibility, of the Denizens of Dread, back 15 minutes after its corpse
// goes. Skipped without a database, or one without them.
func TestRealVirulentBlades(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vblade.db")
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
	if _, err := w.LoadRespawnRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadMoveRules(db); err != nil {
		t.Fatal(err)
	}
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var blades []*NPC
	var wicker *NPC
	for _, npc := range w.NPCs {
		switch {
		case npc.Name == "virulent blade":
			blades = append(blades, npc)
		case npc.ServerID == 118919:
			wicker = npc
		}
	}
	if len(blades) == 0 {
		t.Skip("no virulent blade in this database (tools/virulent_blade_20261114.py not applied)")
	}
	if len(blades) != 6 || wicker == nil {
		t.Fatalf("%d blades, the wicker man %v", len(blades), wicker)
	}
	w.applyBalanceLocked(wicker)
	for _, b := range blades {
		w.applyBalanceLocked(b)
		if b.World != 0 || b.X < 23800 || b.X > 26200 || b.Z < 2000 || b.Z > 4200 || b.Level != 53 || b.Class != 0 {
			t.Errorf("blade %d at world %d (%.0f, %.0f) level %d class %d", b.ServerID, b.World, b.X, b.Z, b.Level, b.Class)
		}
		if b.MaxHP != wicker.MaxHP || (b.tmpl == nil) != (wicker.tmpl == nil) || b.tmpl != nil && b.tmpl.Name != wicker.tmpl.Name {
			t.Errorf("blade %d: %d hit points (template %v), the wicker man %d (%v)", b.ServerID, b.MaxHP, b.tmpl, wicker.MaxHP, wicker.tmpl)
		}
		if got := w.respawnTimeLocked(b.ServerID); got != 15*time.Minute {
			t.Errorf("blade %d respawns after %v", b.ServerID, got)
		}
		if !w.wanders(b) {
			t.Errorf("blade %d does not wander", b.ServerID)
		}
		if f := w.npcFactionLocked(b); f != "Denizens of Dread" {
			t.Errorf("blade %d of faction %q", b.ServerID, f)
		}
	}
	if r, ok := w.moveRuleLocked(blades[0]); !ok || !r.Roam || r.Water != "land" {
		t.Errorf("the blades' move rule %+v %v", r, ok)
	}
	r, ok := w.aggro.ruleFor("virulent blade")
	if !ok || !r.Aggressive || !w.aggro.lookup("virulent blade").seeInvis {
		t.Errorf("the blades' aggro rule %+v %v, sees invisible %v", r, ok, w.aggro.lookup("virulent blade").seeInvis)
	}
	if hits := w.factions.hitsFor("virulent blade"); len(hits) != 1 {
		t.Errorf("the blades' faction hits %+v", hits)
	}
}
