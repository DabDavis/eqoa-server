package entity

import "testing"

// TestStateChannelCarriesWorld pins byte 17, the World field of an ObjectUpdate
// (offset 16 in the 200-byte record, +1 for the isActive prefix).
//
// It was hardcoded to 0 for both players and NPCs, which is invisible while
// world 0 is the only world and undetectable afterwards: the worlds we lift out
// of Tunaria keep Tunaria's coordinates, so a player wrongly reported as being
// in world 0 is at a position world 0 genuinely contains. Nothing downstream
// contradicts it. The symptom that finally surfaced was Harbormaster Machado, a
// world 9 NPC, standing in world 0.
//
// Retail moves this field with the entity. In
// pcaps/sandstorm-worldchange-20260807.pcap the player coaches from Odus
// (world 2) to Freeport (world 0), and the channel 0 update that follows the
// load carries an XOR delta of 0x02 at exactly this byte — 2 XOR 0 — beside
// deltas on position and facing and nothing else.
func TestStateChannelCarriesWorld(t *testing.T) {
	for _, world := range []uint8{0, 2, 8, 9} {
		p := &Player{
			ServerID: 1, Name: "Aafii", Level: 10, ModelID: 0x70D898C6,
			X: 13000, Y: 66.5, Z: 23000, HP: 100, MaxHP: 100,
			World: world,
		}
		if got := SerializePlayerUpdate(100001, p)[17]; got != world {
			t.Errorf("player in world %d serializes byte 17 as %d", world, got)
		}

		n := &NPC{
			ServerID: 2, Name: "Harbormaster Machado", Level: 30,
			ModelID: 0x70D898C6, X: 13000, Y: 66.5, Z: 23000,
			HP: 1000, MaxHP: 1000, World: world,
		}
		if got := SerializeNPCUpdate(100029, n)[17]; got != world {
			t.Errorf("NPC in world %d serializes byte 17 as %d", world, got)
		}
	}
}
