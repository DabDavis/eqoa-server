package entity

import "testing"

// TestNPCTintIndicesAreInRange pins bytes 99-104 to a valid $Tints index.
//
// $Tints (0x004E0C08) has 15 entries and GetTintColor (0x004294E8) does
// `return &Tints[index*4]` with no bounds check; the clamp to 0-14 lives in
// SetArmorSlot, which Sandstorm's S2 patches. Sending 0xFF therefore renders
// correctly on a retail client and renders NPCs with no clothes and invisible
// hands and feet on an S2 client — measured on both, same server, same NPC.
func TestNPCTintIndicesAreInRange(t *testing.T) {
	n := &NPC{
		ServerID: 7, Name: "Guard Jahn", Level: 30, ModelID: 0x70D898C6,
		X: 25334, Y: 56.6, Z: 15686, HP: 1000, MaxHP: 1000, NPCType: 0x0080,
		Torso: 6, Forearm: 6, NPCGloves: 6, NPCLegs: 6, Feet: 6, Head: 7,
	}
	buf := SerializeNPCUpdate(100001, n)
	for off := 99; off <= 104; off++ {
		if buf[off] > 14 {
			t.Errorf("byte %d = %d, outside $Tints 0-14; GetTintColor has no bounds "+
				"check and S2 removes the clamp -> garbage RGBA -> invisible geometry",
				off, buf[off])
		}
	}
}

// TestPlayerTintIndicesAreInRange holds the player path to the same rule. The
// player was never affected only because its serializer leaves 99-104 at zero;
// that is now an assertion rather than an accident.
func TestPlayerTintIndicesAreInRange(t *testing.T) {
	p := &Player{
		ServerID: 42, Name: "Tintcheck", Level: 51, Race: 3, Class: 5,
		HP: 470, MaxHP: 470, ModelID: 0x70D898C6,
		Chest: 7, Bracer: 7, Gloves: 7, Legs: 7, Boots: 7, Helm: 7,
	}
	buf := SerializePlayerUpdate(500000001, p)
	for off := 99; off <= 104; off++ {
		if buf[off] > 14 {
			t.Errorf("byte %d = %d, outside $Tints 0-14", off, buf[off])
		}
	}
}
