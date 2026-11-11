package entity

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// A level gained fills hit points and power (the owner, 2026-11-11: "if you
// ding'd you went full hp and mana"), and the stat channel is due. The
// controls: XP that gains no level leaves them; mastery points at the cap
// leave them; a GM's level set upward fills, downward does not.
func TestADingFillsHitPointsAndPower(t *testing.T) {
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 7111}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1011, CharServerID: 77, Name: "Dinger", Level: 14, Class: 10, XPInLevel: CharXPDict[14] - 10,
		BaseSta: 50, BaseWis: 50, BaseInt: 50}
	p.RecalcStats(true)
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)

	p.HP, p.Power = 3, 2
	w.awardXP(s, p, 5)
	if p.Level != 14 || p.HP != 3 || p.Power != 2 {
		t.Fatalf("the control: no level gained, HP %d power %d (level %d); want 3, 2", p.HP, p.Power, p.Level)
	}
	p.StatsDirty = false
	w.awardXP(s, p, 100)
	if p.Level != 15 {
		t.Fatalf("level %d, want 15", p.Level)
	}
	if p.HP != p.MaxHPNow() || p.Power != p.MaxPowerNow() || p.HP <= 3 || p.Power <= 2 {
		t.Errorf("after the ding HP %d/%d power %d/%d, want full", p.HP, p.MaxHPNow(), p.Power, p.MaxPowerNow())
	}
	if !p.StatsDirty {
		t.Error("the stat channel is not due after the ding")
	}

	// at the cap the experience is mastery points: no fill
	p.Level, p.XPInLevel = MaxLevel, 0
	p.RecalcStats(true)
	p.HP, p.Power = 3, 2
	w.awardXP(s, p, 1000000)
	if p.Level != MaxLevel || p.HP != 3 || p.Power != 2 {
		t.Errorf("at the cap: level %d HP %d power %d, want %d, 3, 2", p.Level, p.HP, p.Power, MaxLevel)
	}

	// a GM's level set
	p.Level = 20
	p.RecalcStats(true)
	p.HP, p.Power = 3, 2
	if !w.SetPlayerLevel(p.CharServerID, 10) || p.HP != 3 || p.Power != 2 {
		t.Errorf("a level set down: HP %d power %d, want 3, 2", p.HP, p.Power)
	}
	if !w.SetPlayerLevel(p.CharServerID, 30) || p.HP != p.MaxHPNow() || p.Power != p.MaxPowerNow() {
		t.Errorf("a level set up: HP %d/%d power %d/%d, want full", p.HP, p.MaxHPNow(), p.Power, p.MaxPowerNow())
	}
}
