package entity

import "testing"

// A creature's channel turns to a corpse at its death as Sandstorm's does:
// byte 26 (CLIENT's VIEntity +0x18) 3, the corpse bit that makes the attack
// button ask for the loot, with its health 0; alive it is 1, selectable.
func TestADeadCreaturesChannelIsACorpse(t *testing.T) {
	w := NewWorld()
	npc := &NPC{ServerID: 9001, Name: "orc pawn", Level: 2, HP: 40, MaxHP: 40, ModelID: 1}
	w.NPCs[npc.ServerID] = npc
	alive := w.serializeEntityForChannel(nil, npc.ServerID, 100001)
	if alive[26] != 1 || alive[27] != 255 {
		t.Fatalf("alive: byte 26 %d, health %d; want 1, 255", alive[26], alive[27])
	}
	npc.Dead, npc.HP = true, 0
	dead := w.serializeEntityForChannel(nil, npc.ServerID, 100001)
	if dead[26] != 3 || dead[27] != 0 || dead[58] != 0x0e {
		t.Fatalf("dead: byte 26 %d, health %d, animation %d; want 3, 0, 14", dead[26], dead[27], dead[58])
	}
}
