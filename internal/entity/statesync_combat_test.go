package entity

import (
	"testing"
	"time"
)

// Petition 2379 (Bolgorf): no battle music on any client. CLIENT and vanilla
// play it while the player's own channel's byte 155 (m_p263) is set; it is set
// while the player swings at a target or within CombatMusicHold of a blow, and
// clear otherwise (the control: a player at rest, a dead one).
func TestChannelByte155IsInCombat(t *testing.T) {
	p := &Player{Name: "Bolgorf", Level: 13, HP: 100, MaxHP: 100}
	if b := SerializePlayerUpdate(1, p)[155]; b != 0 {
		t.Fatalf("at rest: byte 155 is %d", b)
	}
	p.LastCombatTime = time.Now().Add(-2 * time.Second)
	if b := SerializePlayerUpdate(1, p)[155]; b != 1 {
		t.Errorf("2 s after a blow: byte 155 is %d", b)
	}
	p.LastCombatTime = time.Now().Add(-CombatMusicHold - time.Second)
	if b := SerializePlayerUpdate(1, p)[155]; b != 0 {
		t.Errorf("past the hold: byte 155 is %d", b)
	}
	p.AttackTarget, p.AutoAttack = 0x1234, true
	if b := SerializePlayerUpdate(1, p)[155]; b != 1 {
		t.Errorf("swinging: byte 155 is %d", b)
	}
	p.HP = 0
	if b := SerializePlayerUpdate(1, p)[155]; b != 0 {
		t.Errorf("dead: byte 155 is %d", b)
	}
}
