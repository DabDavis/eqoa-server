package entity

import "testing"

// stateTailStart is the first offset no real server ever writes.
//
// Measured, not assumed: every state channel in pcaps/ps2_sandstorm.pcap
// decodes to 162 bytes with nothing non-zero above 161 — channel 0 (a level 51
// character in full gear, named, equipped, tinted) and channels 1-3 (live
// NPCs). If any field up there were real, a character that complete would have
// populated it.
const stateTailStart = 162

// TestPlayerStateTailIsZero and TestNPCStateTailIsZero pin that we do not write
// into the client's entity struct past the end of the layout retail uses.
//
// A duplicate "C# tail" block (Level, Movement, NameColor, NamePlate, Race,
// Class, NPCType, Pattern, Online and a second "trsq" signature at 179-200)
// was written here for compatibility with the C# reference server. The PS2
// entered the world and then could not move, showed a full buff bar, and
// reported spells it had not learned.
func TestPlayerStateTailIsZero(t *testing.T) {
	p := &Player{
		ServerID: 42, Name: "Tailcheck", Level: 51, Race: 3, Class: 5,
		X: 25335, Y: 56.6, Z: 15690, Facing: 3.14,
		HP: 470, MaxHP: 470, ModelID: 0x70D898C6,
		Chest: 7, Bracer: 7, Gloves: 7, Legs: 7, Boots: 7, Helm: 7,
		Primary: 1234, Secondary: 5678, Shield: 9012, Robe: 3456,
	}
	assertTailZero(t, "player", SerializePlayerUpdate(500000001, p))
}

func TestNPCStateTailIsZero(t *testing.T) {
	n := &NPC{
		ServerID: 7, Name: "Guard Jahn", Level: 30, ModelID: 0x70D898C6,
		X: 25334, Y: 56.6, Z: 15686, HP: 1000, MaxHP: 1000, NPCType: 0x0080,
	}
	assertTailZero(t, "npc", SerializeNPCUpdate(100001, n))
}

// TestVisibilityChannelTailIsZero covers the path that actually reaches the
// wire for everything except the player themself.
//
// The two tests above assert the SERIALIZERS and passed throughout, while
// serializeEntityForChannel took their output and wrote into it afterwards —
// byte 146 (setting the NPC bit on a player) and bytes 185-187 (the C# tail).
// Once EntityStateSize dropped from 201 to 162 the second write was out of
// range, and it panicked the tick goroutine the first time a second player
// became visible. Testing the serializer is not testing what is sent.
func TestVisibilityChannelTailIsZero(t *testing.T) {
	w := NewWorld()
	p := &Player{
		ServerID: 42, Name: "Tailcheck", Level: 51, Race: 3, Class: 5,
		X: 25335, Y: 56.6, Z: 15690, Facing: 3.14,
		HP: 470, MaxHP: 470, ModelID: 0x70D898C6,
		Chest: 7, Bracer: 7, Gloves: 7, Legs: 7, Boots: 7, Helm: 7,
	}
	n := &NPC{
		ServerID: 7, Name: "Guard Jahn", Level: 30, ModelID: 0x70D898C6,
		X: 25334, Y: 56.6, Z: 15686, HP: 1000, MaxHP: 1000, NPCType: 0x0080,
	}
	w.Players[EntityID(p.ServerID)] = p
	w.NPCs[EntityID(n.ServerID)] = n

	pb := w.serializeEntityForChannel(nil, EntityID(p.ServerID), 500000042)
	assertTailZero(t, "visibility/player", pb)
	nb := w.serializeEntityForChannel(nil, EntityID(n.ServerID), 100007)
	assertTailZero(t, "visibility/npc", nb)

	// And the distinction the panicking code was trying to make: a player's
	// NPCType high half is ZERO, an NPC's is not. Getting this backwards is
	// what ran the wrong appearance path for both.
	if pb[146] != 0x00 {
		t.Errorf("player NPCType high half = %#02x, want 0x00 — 0x80 is the NPC bit", pb[146])
	}
	if pb[145] != byte(p.Class) {
		t.Errorf("player NPCType low half = %#02x, want the class %#02x", pb[145], p.Class)
	}
	if nb[145] == 0x00 && nb[146] == 0x00 {
		t.Error("NPC NPCType is 0x0000, which marks it as a player")
	}
}

func assertTailZero(t *testing.T, what string, buf []byte) {
	t.Helper()

	// The LENGTH is the invariant, not just the contents.
	//
	// This check used to be `len(buf) != EntityStateSize` with EntityStateSize
	// at 201, so it passed while the serializer shipped a 39-byte tail. Those
	// bytes being zero does not keep them off the wire: RunLengthEncode encodes
	// a trailing zero run instead of stopping at the last non-zero byte, so the
	// client received them and wrote them into its entity struct regardless.
	// Comparing against the measured retail size is what makes this test able
	// to fail; comparing against our own constant never could.
	if len(buf) != stateTailStart {
		t.Fatalf("%s buffer is %d bytes, want %d (every object channel in "+
			"pcaps/ps2_sandstorm.pcap decodes to exactly %d) — anything longer "+
			"is sent and overruns the client's entity struct",
			what, len(buf), stateTailStart, stateTailStart)
	}
	var dirty []int
	for i := stateTailStart; i < len(buf); i++ {
		if buf[i] != 0 {
			dirty = append(dirty, i)
		}
	}
	if len(dirty) > 0 {
		t.Errorf("%s writes into the client's entity struct past offset %d, at %v — "+
			"retail leaves every one of these zero", what, stateTailStart-1, dirty)
	}

	// The layout below the tail must still be populated, or this test would
	// also pass on a serializer that writes nothing at all.
	if buf[0] != 1 {
		t.Errorf("%s: isActive flag not set", what)
	}
	sig := string(buf[158:162])
	if sig != "tsrq" {
		t.Errorf("%s: end signature at 158-161 = %q, want \"tsrq\"", what, sig)
	}
}
