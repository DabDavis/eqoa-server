package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Watchman's moments (bossmoments.go): a self dot off its own hit points,
// a raid heal onto players of its hate list, its target's power to 0, each on
// its clock from the fight's start.
func TestABossesMomentsComeOnTheirClocks(t *testing.T) {
	list := []database.TemplateAbility{
		{Kind: database.AbilitySelfDoT, Every: 6, Damage: 3000},
		{Kind: database.AbilityRaidHeal, Every: 30, First: 10, Damage: 10000, Count: 3},
		{Kind: database.AbilityPowerZero, Every: 60},
	}
	if err := database.CheckTemplateAbilities(list); err != nil {
		t.Fatal(err)
	}
	if err := database.CheckTemplateAbilities([]database.TemplateAbility{{Kind: database.AbilityRaidHeal, Every: 30, Damage: 10}}); err == nil {
		t.Fatal("a raid heal of no count passed")
	}
	f := newAggroFix(t, 60)
	f.p.Power, f.p.MaxPower, f.p.HP = 800, 800, 500
	other := &Player{ServerID: 1002, CharServerID: 78, Name: "Other", Level: 60, HP: 100, MaxHP: 20000, X: 1005, Y: 10, Z: 1000}
	f.w.Players[other.ServerID] = other
	w := f.npc(3100, "The Watchman", 62, creationBody, 1010, 1000)
	w.HP, w.MaxHP = 300000, 300000
	w.tmpl = &ClassTemplate{moments: list}
	w.CurrentEnemy = f.p.ServerID
	w.Threats = map[EntityID]int64{f.p.ServerID: 10, other.ServerID: 5}
	now := f.now
	fight := w.fight()
	f.w.bossMomentsLocked(w, fight, now)
	if w.HP != 297000 || f.p.Power != 0 {
		t.Fatalf("at the pull: HP %d (want 297000), power %d (want 0)", w.HP, f.p.Power)
	}
	if other.HP != 100 {
		t.Fatalf("the raid heal came before its first: %d", other.HP)
	}
	f.w.bossMomentsLocked(w, fight, now.Add(3*time.Second))
	if w.HP != 297000 {
		t.Fatalf("the self dot came again before its every: %d", w.HP)
	}
	f.p.Power = 500
	f.w.bossMomentsLocked(w, fight, now.Add(10*time.Second))
	if w.HP != 294000 || other.HP != 10100 || f.p.HP != 1000 || f.p.Power != 500 {
		t.Fatalf("at 10 s: HP %d, other %d, player %d, power %d", w.HP, other.HP, f.p.HP, f.p.Power)
	}
	w.HP = 2000
	f.w.bossMomentsLocked(w, fight, now.Add(20*time.Second))
	if w.HP != 1 {
		t.Fatalf("the self dot took it to %d, want 1 (never dead)", w.HP)
	}
}
