package entity

import "testing"

// TestNPCTypeWireBytes pins bytes 145-146, the field that decides whether the
// client treats an entity as an NPC or a player.
//
// This has broken twice with the same visible symptom — naked NPCs with
// invisible hands and feet — and no error anywhere, because a player-path
// entity asks VICSpriteCust for racial skin and clothes an NPC model does not
// carry. First the field was written one byte late; then it was written with
// the right offset but the wrong byte order.
//
// The expectations are retail's own, read out of packets.db's live capture:
// merchants carry 145=00 146=81, and the npcs.npc_type column stores those
// same entities byte-reversed as 0x0081.
func TestNPCTypeWireBytes(t *testing.T) {
	cases := []struct {
		name    string
		dbValue int
		want145 byte
		want146 byte
	}{
		{"merchant (retail 0x8100)", 0x0081, 0x00, 0x81},
		{"plain NPC (retail 0x8000)", 0x0080, 0x00, 0x80},
		{"sign post (retail 0x8001)", 0x0180, 0x01, 0x80},
		// Retail sends 0x0000 for 16 of 49 captured NPCs (City Cat, guards) and
		// they render clothed, so zero passes through untouched.
		{"no type in DB -> passed through", 0x0000, 0x00, 0x00},
	}
	for _, c := range cases {
		n := &NPC{
			ServerID: 7, Name: "Guard Jahn", Level: 30, ModelID: 0x70D898C6,
			X: 25334, Y: 56.6, Z: 15686, HP: 1000, MaxHP: 1000,
			NPCType: c.dbValue,
		}
		buf := SerializeNPCUpdate(100001, n)
		if buf[145] != c.want145 || buf[146] != c.want146 {
			t.Errorf("%s: npc_type=0x%04X -> wire 145=%02X 146=%02X, want 145=%02X 146=%02X",
				c.name, c.dbValue, buf[145], buf[146], c.want145, c.want146)
		}
	}
}
