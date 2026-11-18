package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Ahriaana the Ultimate's port of the one she fights (the owner's Evil Melee
// epic, 2026-11-18, the forum's t155: "ports those with aggro off the
// island"; board 615: to its bind point, pets never): a player she fights is
// sent home and forgotten, so she turns to the next on her list; a pet she
// fights is never sent, and the moment passes; none before its first.
func TestAPortFoeSendsHerTankHomeAndNeverAPet(t *testing.T) {
	list := []database.TemplateAbility{{Kind: database.AbilityPortFoe, Name: "Ultimate Banishment", Every: 30, First: 30}}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []database.TemplateAbility{
		{Kind: database.AbilityPortFoe, Every: 30, To: []float64{4, 1, 2}},                  // three numbers
		{Kind: database.AbilityPortFoe, Every: 30, To: []float64{4.5, 1, 2, 3}},             // a world of a half
		{Kind: database.AbilityPortNear, Every: 30, Radius: 100, To: []float64{4, 1, 2, 3}}, // a portnear's to
		{Kind: database.AbilityNuke, Every: 30, To: []float64{4, 1, 2, 3}},                  // a nuke's
		{Kind: database.AbilityPortFoe, Every: 30, Radius: 50},                              // a radius
	} {
		if err := database.CheckTemplateAbilities([]database.TemplateAbility{bad}); err == nil {
			t.Errorf("passed: %+v", bad)
		}
	}
	f := newAggroFix(t, 60)
	a := f.npc(3300, "Ahriaana the Ultimate", 66, creationBody, 1005, 1000)
	a.HP, a.MaxHP = 1000000, 1000000
	a.tmpl = &ClassTemplate{moments: list}
	pet := f.npc(3301, "Bonecrusher", 60, creationBody, 1004, 1000)
	a.CurrentEnemy = f.p.ServerID
	a.Threats = map[EntityID]int64{f.p.ServerID: 100, pet.ServerID: 10}
	fight := a.fight()
	f.w.bossMomentsLocked(a, fight, f.now)
	if len(f.w.gates.due) != 0 {
		t.Fatal("a port before its first")
	}
	f.w.bossMomentsLocked(a, fight, f.now.Add(30*time.Second))
	if len(f.w.gates.due) != 1 {
		t.Fatalf("the port: %+v", f.w.gates.due)
	}
	if tr := f.w.gates.due[0]; tr.P != f.p || tr.Home != f.p.ServerID {
		t.Fatalf("sent %+v, want her tank to its own bind point", tr)
	}
	if _, ok := a.Threats[f.p.ServerID]; ok || a.CurrentEnemy != pet.ServerID {
		t.Fatalf("after the port: hate %v, on %d; want the tank forgotten and the pet her enemy", a.Threats, a.CurrentEnemy)
	}
	f.w.gates.due = nil
	f.w.bossMomentsLocked(a, fight, f.now.Add(60*time.Second))
	if len(f.w.gates.due) != 0 || a.CurrentEnemy != pet.ServerID {
		t.Fatalf("a pet holding her: sent %+v, on %d", f.w.gates.due, a.CurrentEnemy)
	}
}

// With a place of its own (to) the port lands there, not at the bind point.
func TestAPortFoeWithAPlaceLandsThere(t *testing.T) {
	list := []database.TemplateAbility{{Kind: database.AbilityPortFoe, Every: 30, To: []float64{4, 5000.09, 333.54, 4998.23}}}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	f := newAggroFix(t, 60)
	a := f.npc(3300, "Ahriaana the Ultimate", 66, creationBody, 1005, 1000)
	a.tmpl = &ClassTemplate{moments: list}
	a.CurrentEnemy = f.p.ServerID
	a.Threats = map[EntityID]int64{f.p.ServerID: 100}
	f.w.bossMomentsLocked(a, a.fight(), f.now)
	if len(f.w.gates.due) != 1 {
		t.Fatalf("the port: %+v", f.w.gates.due)
	}
	if tr := f.w.gates.due[0]; tr.Home != 0 || tr.To.World != 4 || tr.To.X != 5000.09 || tr.To.Z != 4998.23 {
		t.Fatalf("sent %+v, want the place", tr)
	}
	if a.CurrentEnemy != 0 || len(a.Threats) != 0 {
		t.Fatalf("hate %v, on %d: want none left", a.Threats, a.CurrentEnemy)
	}
}
