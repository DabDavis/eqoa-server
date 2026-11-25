package protocol

// The harmful effects a player has put on the creatures its client draws, for the small
// icons a client draws under their name plates (entity/npcdebuffs.go, handlers/npcdebuffs.go;
// Heirloom petition 2997, Cucumber, 2026-11-25: "Debuffs cast on evil NPCs (combat) give an
// icon under their ID plate with remaining time on the debuff (so you know when to refresh)
// or at the very least the icon so you know when the debuff is no longer active").
// CLIENT's name plates (VINameSprite) carry a name and nothing else, and no message of the
// game's carries a creature's effects. Not retail, past the group effects' 0x7EC0-0x7EC1
// (the stat figures' 0x7E84-0x7E85 and the hardcore 0x7EB0-0x7EB2 untouched). A PS2
// ignores the offer (CLIENT's dispatcher has no case above 0x1402 and no default) and is
// sent none.
const (
	// OpcodeNPCDebuffs, both ways, a u32: the server offers it (1) once the player is in the
	// world; a client that draws the icons answers 1 to be sent the set
	// (OpcodeNPCDebuffsData) and 0 to stop, at once and whenever that changes.
	OpcodeNPCDebuffs GameOpcode = 0x7EC2
	// OpcodeNPCDebuffsData, server to a client that answered 1, at once and whenever the set
	// changes (an effect put on, cast again, run out or taken off, a creature come onto or
	// gone from the client's slots): a u8 count of creatures, then each one's u32 id (as the
	// client's entity channels name it), a u8 count of its effects and per effect its u32
	// icon (a DictID, as channel 0x43's), a u8 whose (0 the player's own, 1 its pet's, 2 its
	// merc's), its u32 milliseconds left as the server made the message (0: no end) and its
	// name (a u8 byte count and UTF-8, at most 63 bytes), in the order they went on. Only
	// harmful effects the player, its pet or its mercs put on, at most 12 a creature; the
	// target first, then the nearest, as many as fit a message. Each is the whole set: a
	// creature it leaves out has none of the player's.
	OpcodeNPCDebuffsData GameOpcode = 0x7EC3
)
