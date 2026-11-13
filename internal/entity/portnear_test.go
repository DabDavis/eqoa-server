package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Quintashi's death touch (the owner, 2026-11-13): as the fight begins the one
// he fights is killed outright; not again until its every.
func TestADeathTouchKillsTheFirstTarget(t *testing.T) {
	list := []database.TemplateAbility{{Kind: database.AbilityDeathTouch, Every: 3600}}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	f := newAggroFix(t, 60)
	f.p.HP, f.p.MaxHP = 9000, 9000
	q := f.npc(3200, "Quintashi", 66, creationBody, 1005, 1000)
	q.HP, q.MaxHP = 1000000, 1000000
	q.tmpl = &ClassTemplate{moments: list}
	q.CurrentEnemy = f.p.ServerID
	q.Threats = map[EntityID]int64{f.p.ServerID: 10}
	f.w.bossMomentsLocked(q, q.fight(), f.now)
	if f.p.HP != 0 {
		t.Fatalf("the first target at %d after the death touch", f.p.HP)
	}
}

// Quintashi's port (the owner, 2026-11-13: "he will port players close to the
// mobs in the chamber so they get attacked by them", every 3 minutes): a
// player of his hate list sent beside one of the named creatures within his
// radius, and the creatures round it set on them; none before its first, and
// a creature not of its names never chosen.
func TestAPortSendsAPlayerAmongTheChambersCreatures(t *testing.T) {
	list := []database.TemplateAbility{{Kind: database.AbilityPortNear, Every: 180, First: 180, Radius: 450,
		Foes: []string{"enchanting siren"}}}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilityPortNear, Every: 180}}); err == nil {
		t.Fatal("a port of no radius passed")
	}
	f := newAggroFix(t, 60)
	q := f.npc(3200, "Quintashi", 66, creationBody, 1005, 1000)
	q.tmpl = &ClassTemplate{moments: list}
	q.CurrentEnemy = f.p.ServerID
	q.Threats = map[EntityID]int64{f.p.ServerID: 10}
	s1 := f.npc(3201, "enchanting siren", 60, creationBody, 1300, 1000)
	s2 := f.npc(3202, "enchanting siren", 60, creationBody, 1310, 1005)
	other := f.npc(3203, "merman", 46, creationBody, 1100, 1000)
	fight := q.fight()
	f.w.bossMomentsLocked(q, fight, f.now)
	if len(f.w.gates.due) != 0 {
		t.Fatal("a port before its first")
	}
	f.w.bossMomentsLocked(q, fight, f.now.Add(180*time.Second))
	if len(f.w.gates.due) != 1 || f.w.gates.due[0].P != f.p {
		t.Fatalf("the port: %+v", f.w.gates.due)
	}
	to := f.w.gates.due[0].To
	if to.X < 1290 {
		t.Fatalf("sent to (%.0f, %.0f), not beside a siren", to.X, to.Z)
	}
	if s1.Threats[f.p.ServerID] == 0 || s2.Threats[f.p.ServerID] == 0 || other.Threats[f.p.ServerID] != 0 {
		t.Fatalf("set on them: sirens %v %v, merman %v", s1.Threats, s2.Threats, other.Threats)
	}
}
