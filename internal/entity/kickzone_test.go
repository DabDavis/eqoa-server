package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petition 1956, the Anvil of Sarek: a kickzone of 19.9 kicks every player within it for 10,000
// every second, in or out of a fight, through anything; the first to step in at once; one at 20
// across, one 30 below and a GM flying are spared; a kick that takes the last hit point kills.
func TestAKickZoneKicksWhoStepsIn(t *testing.T) {
	zone := database.TemplateAbility{Kind: database.AbilityKickZone, Radius: 19.9, Damage: 10000, Every: 1}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{zone}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []database.TemplateAbility{
		{Kind: database.AbilityKickZone, Damage: 10000},
		{Kind: database.AbilityKickZone, Radius: 20},
		{Kind: database.AbilityKickZone, Radius: 20, Damage: 10, Every: 0.1},
		{Kind: database.AbilityKickZone, Radius: 20, Damage: 10, Chance: 50},
	} {
		if database.CheckTemplateAbilities([]database.TemplateAbility{bad}) == nil {
			t.Errorf("%+v passed", bad)
		}
	}
	f := newAggroFix(t, 60)
	f.p.HP, f.p.MaxHP = 25000, 25000
	anvil := f.npc(3300, "Anvil of Sarek", 65, creationBody, 1000, 1000)
	anvil.NPCType = 0x80
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	anvil.tmpl = &ClassTemplate{kickZones: []database.TemplateAbility{zone}}
	f.w.kickZoneWatchLocked(anvil)
	at := func(ms int) time.Time { return f.now.Add(time.Duration(ms) * time.Millisecond) }

	f.p.X, f.p.Z = 1000, 1020 // 20 off
	f.w.kickZoneSystem(at(0))
	f.p.X, f.p.Y, f.p.Z = 1000, -25, 1010 // 10 across, 35 below
	f.w.kickZoneSystem(at(100))
	f.p.Y = 10
	f.p.Flying = true
	f.w.kickZoneSystem(at(200))
	if f.p.HP != 25000 {
		t.Fatalf("outside it, below it, flying: HP %d", f.p.HP)
	}
	f.p.Flying, f.p.flyGrace = false, time.Time{}
	f.p.Z = 1019.8
	f.w.kickZoneSystem(at(300))
	if f.p.HP != 15000 {
		t.Fatalf("stepping in: HP %d, want 15000", f.p.HP)
	}
	f.w.kickZoneSystem(at(800))
	if f.p.HP != 15000 {
		t.Fatalf("half a second on: HP %d", f.p.HP)
	}
	f.w.kickZoneSystem(at(1300))
	if f.p.HP != 5000 {
		t.Fatalf("a second on: HP %d, want 5000", f.p.HP)
	}
	f.w.kickZoneSystem(at(2300))
	if f.p.HP != 0 || !f.p.IsDead() {
		t.Fatalf("the third kick: HP %d, dead %v", f.p.HP, f.p.IsDead())
	}
	// gone from the world: no longer watched
	delete(f.w.NPCs, anvil.ServerID)
	f.w.kickZoneSystem(at(5000))
	if len(f.w.kickZones) != 0 {
		t.Fatal("a creature gone is still watched")
	}
}
