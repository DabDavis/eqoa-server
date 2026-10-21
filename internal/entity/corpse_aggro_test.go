package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// TestAISystemDropsAggroOnCorpse pins the guard that stops an NPC from
// attacking a player who is already at 0 HP.
//
// Without it, every aiSystem pass re-attacked the corpse and queued three more
// reliable messages (CastingCombat, DamageIndication, CharacterDied) that the
// client had to ack in order. On the PS2 this reached ~40 reliable messages a
// second, the client stopped acking, and the game looked frozen a few seconds
// after world entry.
func TestAISystemDropsAggroOnCorpse(t *testing.T) {
	w := NewWorld()
	// npcAttackPlayer looks the session up to send combat feedback; an empty
	// manager is enough (the attack itself still lands on the Player struct).
	w.SessionManager = session.NewManager()

	player := &Player{ServerID: 42, HP: 0, MaxHP: 47, X: 100, Z: 100}
	w.Players[player.ServerID] = player

	npc := &NPC{ServerID: 7, Name: "Guard Jahn", X: 102, Z: 100, CurrentEnemy: player.ServerID}
	npc.Threats = map[EntityID]int64{player.ServerID: 500}
	w.NPCs[npc.ServerID] = npc

	w.aiSystem(timeNowForTest())

	if npc.CurrentEnemy != 0 {
		t.Errorf("NPC still targets the corpse (CurrentEnemy=%d), want 0", npc.CurrentEnemy)
	}
	if len(npc.Threats) != 0 {
		t.Errorf("threat table still holds %d entries, want 0", len(npc.Threats))
	}

	// A live player at the same spot must still be attacked, or the guard has
	// simply disabled combat.
	player.HP = player.MaxHP
	npc.CurrentEnemy = player.ServerID
	npc.Threats = map[EntityID]int64{player.ServerID: 500}
	w.aiSystem(timeNowForTest())
	w.aiSystem(timeNowForTest().Add(500 * time.Millisecond)) // the round's first hit

	if npc.CurrentEnemy != player.ServerID {
		t.Errorf("NPC dropped aggro on a living player (CurrentEnemy=%d), want %d",
			npc.CurrentEnemy, player.ServerID)
	}
	if player.HP == player.MaxHP {
		t.Error("living player took no damage — the corpse guard is too broad")
	}
}

// TestNPCAttackPlayerIgnoresCorpse covers the same rule at the call it protects,
// so a future caller that skips aiSystem's check cannot reopen the loop.
func TestNPCAttackPlayerIgnoresCorpse(t *testing.T) {
	w := NewWorld()
	// npcAttackPlayer looks the session up to send combat feedback; an empty
	// manager is enough (the attack itself still lands on the Player struct).
	w.SessionManager = session.NewManager()

	player := &Player{ServerID: 42, HP: 0, MaxHP: 47, X: 100, Z: 100}
	w.Players[player.ServerID] = player
	npc := &NPC{ServerID: 7, Name: "Guard Jahn", X: 102, Z: 100}
	w.NPCs[npc.ServerID] = npc

	before := player.LastCombatTime
	w.npcAttackPlayer(npc, player.ServerID, timeNowForTest())

	if player.LastCombatTime != before {
		t.Error("npcAttackPlayer processed an attack on a corpse")
	}
}

func timeNowForTest() time.Time { return time.Unix(1_700_000_000, 0) }
