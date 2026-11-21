package entity

import "testing"

// Petition 5391: a member's group record carries its world in byte 6 (CLIENT's
// m_p0C), which DrawPatch matches to the player's world before it puts a
// member out of view on the compass; it was always 0, Tunaria's.
func TestTheGroupRecordCarriesTheWorld(t *testing.T) {
	r := make([]byte, 13)
	groupRecord(r, 7, groupView{hp: 50, top: 100, x: 8792, y: 54, z: 7355, world: 1})
	if r[6] != 1 {
		t.Fatalf("world byte %d, want 1 (Odus)", r[6])
	}
	if x := int16(uint16(r[7]) | uint16(r[8])<<8); x != 8792 {
		t.Fatalf("x %d", x)
	}
	groupRecord(r, 7, groupView{world: 0})
	if r[6] != 0 {
		t.Fatalf("the control: Tunaria's 0, got %d", r[6])
	}
}
