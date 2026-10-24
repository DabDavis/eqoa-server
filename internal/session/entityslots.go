package session

// Entity slots: the object state channels a client keeps an entity on.
//
// CLIENT keeps 24 (VIClient::m_entities[24]; ProcessStateChannel, 0x00BD3C18,
// hands only `type < 24` to ChannelEntity), on the state channels 0x00-0x17,
// the player's own entity among them. DRDP carries 248 state channels
// (connection_allocate_state_channels: message types 0x00-0xF7, 0xF8 and up
// being its transports), and of the rest CLIENT reads 0x40 (the group), 0x42
// (stats), 0x43 (effects) and 0x44-0x6B (Channel44's forty records) as other
// things and ignores the others.
//
// A client that keeps more slots (eqoa-rust-client-vanilla's NPC/PC Display)
// asks for them with OpcodeEntitySlots once the server has offered them, and
// slot n travels on EntityChannel(n): slots 24-63 on 0x18-0x3F and 64-203 on
// 0x6C-0xF7, so no slot lands on a channel CLIENT reads as something else.
// A session that never asks (every PS2) keeps RetailEntitySlots: channels
// 0x00-0x17, as before.
const (
	// RetailEntitySlots is CLIENT's m_entities[24]: channels 0x00-0x17.
	RetailEntitySlots = 24
	// MaxEntitySlots is every state channel CLIENT reads as nothing else:
	// 0x00-0x3F and 0x6C-0xF7.
	MaxEntitySlots = 0x40 + (0xF8 - 0x6C)
)

// EntityChannel is the state channel (the DRDP message type) entity slot
// slot travels on, for 0 <= slot < MaxEntitySlots.
func EntityChannel(slot int) byte {
	if slot < 0x40 {
		return byte(slot)
	}
	return byte(slot - 0x40 + 0x6C)
}

// EntitySlot is the entity slot state channel ch carries, or -1 when it
// carries none (0x40-0x6B, and 0xF8 and up, which are not state channels).
func EntitySlot(ch byte) int {
	switch {
	case ch < 0x40:
		return int(ch)
	case ch >= 0x6C && ch < 0xF8:
		return int(ch) - 0x6C + 0x40
	}
	return -1
}

// ClampEntitySlots is n within RetailEntitySlots and limit, limit itself
// within RetailEntitySlots and MaxEntitySlots.
func ClampEntitySlots(n, limit int) int {
	return max(RetailEntitySlots, min(n, limit, MaxEntitySlots))
}

// The view distance: how far round the player the visibility system looks
// for a session's entities. A client that keeps more than CLIENT's draws its
// NPCs and players farther than the game does (eqoa-rust-client-vanilla's
// NPC/PC View Distance; the game hides a player race's body past 100) and
// asks for its distance with the slots.
const (
	// ViewDistance is how far the visibility system looks for a client that
	// never asked (every PS2), and the least it looks for any.
	ViewDistance = 300
	// MaxViewDistance is the most a client may ask: the vanilla client keeps
	// the world's zones read to 1050 round the player.
	MaxViewDistance = 1000
)

// ClampViewDistance is d within ViewDistance and limit, limit itself within
// ViewDistance and MaxViewDistance.
func ClampViewDistance(d, limit int) int {
	return max(ViewDistance, min(d, limit, MaxViewDistance))
}
