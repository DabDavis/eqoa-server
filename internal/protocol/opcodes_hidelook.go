package protocol

// A worn item's look hidden (the owner, 2026-11-11: "lets add a hide display
// option to equipped armor/weapon, still equipped, but no model displayed";
// entity/hidelook.go, handlers/hidelook.go). A character marks an equip slot
// hidden: what is worn there keeps its stats and effects, and every client is
// sent the slot as bare (the entity channel, the character list, the capes).
// A client that can set it is offered it; a PS2 ignores the offer and sees
// the hidden looks as everyone does.
const (
	// OpcodeHideLookOffer is not retail: server to client once the player is
	// in the world. Little-endian: a u8 layout (1), then a u32, the equip
	// slots (CLIENT's equip locations, 0-21, and the cape's 22) whose look the
	// character hides, bit n the slot n. A PS2 ignores it (CLIENT's dispatcher
	// has no case above 0x1402 and no default).
	OpcodeHideLookOffer GameOpcode = 0x7E6C
	// OpcodeHideLookSet is a client's choice, client to server, only after
	// the offer: a u8, the equip slot, then a u8, 1 to hide its look, 0 to
	// show it. The server answers OpcodeHideLookState.
	OpcodeHideLookSet GameOpcode = 0x7E6D
	// OpcodeHideLookState is the server's answer to OpcodeHideLookSet: a u32,
	// the hidden equip slots as the offer's, after the change.
	OpcodeHideLookState GameOpcode = 0x7E6E
)
