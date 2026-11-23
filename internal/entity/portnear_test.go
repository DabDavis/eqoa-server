package entity

import (
	"net"
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

// Dynmath the Tyrant's port (Live petition 5500, the owner 2026-11-22: he
// "will port whoever has aggro into his cadre of minions"): a portnear with top
// sends the one he fights, the top of his hate list, never the other player on
// it, beside one of his minions, and his minions round it are set on that
// player; his line is said round him. The controls: one he fights that is a pet
// sends no one; a top with a count past one, and a say on another kind, are
// refused.
func TestAPortWithTopSendsTheOneHeFightsAmongHisMinions(t *testing.T) {
	list := []database.TemplateAbility{{Kind: database.AbilityPortNear, Every: 45, First: 30, Radius: 60, Top: true,
		Foes: []string{"minion of Dynmath"}, Name: "Grasp of the Dead", Say: "Join my dead!"}}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]database.TemplateAbility{
		{{Kind: database.AbilityPortNear, Every: 45, Radius: 60, Top: true, Count: 2}},
		{{Kind: database.AbilityPortFoe, Every: 45, Top: true}},
		{{Kind: database.AbilityPortFoe, Every: 45, Say: "Begone"}},
	} {
		if err := database.CheckTemplateAbilities(bad); err == nil {
			t.Errorf("%+v passed", bad[0])
		}
	}
	f := newAggroFix(t, 60)
	// a second player on his list, with more hate than the one he fights
	sess2 := f.w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 10), Port: 7001}, 2, 1, 1, 2, true)
	p2 := &Player{ServerID: 1002, CharServerID: 78, Name: "Other", Level: 60, HP: 1000, MaxHP: 1000, X: 1005, Y: 10, Z: 1000}
	f.w.Players[p2.ServerID] = p2
	f.w.SpatialIndex.Insert(p2.ServerID, p2.X, p2.Z)
	sess2.SetECSEntity(p2)
	sess2.SetCharacterInWorld(true)
	sess2.SetPlayerObjectID(0x1424A1D1)
	d := f.npc(3300, "Dynmath the Tyrant", 60, creationBody, 1010, 1000)
	d.HP, d.MaxHP = 400000, 400000
	d.tmpl = &ClassTemplate{moments: list}
	d.CurrentEnemy = f.p.ServerID
	d.Threats = map[EntityID]int64{f.p.ServerID: 10, p2.ServerID: 50}
	m1 := f.npc(3301, "minion of Dynmath", 57, creationBody, 1040, 1010)
	m2 := f.npc(3302, "minion of Dynmath", 57, creationBody, 1045, 1015)
	other := f.npc(3303, "wandering ghost", 50, creationBody, 1020, 990)
	fight := d.fight()
	f.w.bossMomentsLocked(d, fight, f.now)
	if len(f.w.gates.due) != 0 {
		t.Fatal("a port before its first")
	}
	f.w.bossMomentsLocked(d, fight, f.now.Add(30*time.Second))
	if len(f.w.gates.due) != 1 || f.w.gates.due[0].P != f.p {
		t.Fatalf("the port: %+v, want the one he fights alone", f.w.gates.due)
	}
	to := f.w.gates.due[0].To
	if to.X < 1039 {
		t.Fatalf("sent to (%.0f, %.0f), not beside a minion", to.X, to.Z)
	}
	if m1.Threats[f.p.ServerID] == 0 || m2.Threats[f.p.ServerID] == 0 || other.Threats[f.p.ServerID] != 0 ||
		m1.Threats[p2.ServerID] != 0 {
		t.Fatalf("set on them: minions %v %v, ghost %v", m1.Threats, m2.Threats, other.Threats)
	}

	// the control: he fights a pet: no one goes
	f.w.gates.due = nil
	d.CurrentEnemy = 9999
	f.w.bossMomentsLocked(d, fight, f.now.Add(80*time.Second))
	if len(f.w.gates.due) != 0 {
		t.Fatalf("a port while he fights a pet: %+v", f.w.gates.due)
	}
}
