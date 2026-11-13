package entity

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The Anvil of Sarek over a real database with tools/anvil_1956_20261113.py applied
// (EQOA_ANVIL_DB names it; a copy is opened): its template loads, and the Anvil's NPC is
// watched with its 19.9 kick zone. Skipped without one.
func TestTheAnvilsKickZoneLoads(t *testing.T) {
	src := os.Getenv("EQOA_ANVIL_DB")
	if src == "" {
		t.Skip("EQOA_ANVIL_DB names a database with the Anvil's tool applied")
	}
	t.Setenv("EQOA_REAL_DB", src)
	w := realWorld(t)
	db, err := database.NewSQLite(src)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		t.Log(p)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	n := w.NPCs[120968]
	if n == nil || n.Name != "Anvil of Sarek" {
		t.Fatalf("no Anvil: %+v", n)
	}
	zs := n.tmpl.kickZonesOf()
	if len(zs) != 1 || zs[0].Radius != 19.9 || zs[0].Damage != 10000 || w.kickZones[n.ServerID] == nil {
		t.Fatalf("its zones %+v, watched %v", zs, w.kickZones[n.ServerID] != nil)
	}
}
