package protocol

// Quest marks over NPCs' heads (the GM Jibrish's petition 1813, 2026-11-12:
// "Add the ability for us to add yellow ! and ? over NPCs in game, visible to
// the player, if they toggle it on or we turn it on by default";
// handlers/questheadmarks.go, entity/questheadmarks.go). A "!" over an NPC
// with a quest the player may take now, a "?" over one where a quest's step
// can be done now (a talk, or a show or give with the items held, or a hand-in
// the player holds the items for). Which quests show them is the server's
// (quests.markers, table questmarks). A client that draws them is offered
// them; a PS2 ignores the offer and is sent nothing.
const (
	// OpcodeQuestHeadMarksOffer is not retail: server to client once the
	// player is in the world, a u8, the layout of OpcodeQuestHeadMarks (1). A
	// PS2 ignores it (CLIENT's dispatcher has no case above 0x1402 and no
	// default).
	OpcodeQuestHeadMarksOffer GameOpcode = 0x7E61
	// OpcodeQuestHeadMarksAnswer is a client's answer to the offer, client to
	// server, at once and whenever its setting changes: a u8, 1 to be sent
	// the marks, 0 to stop. Nothing is answered.
	OpcodeQuestHeadMarksAnswer GameOpcode = 0x7E62
	// OpcodeQuestHeadMarks is the marks over the NPCs the client has in view,
	// server to a client that answered 1, the whole set each time it changes
	// (an NPC comes into view or goes, a quest is taken or a step done, an
	// item gained, a level): little-endian, a u16 count, then each NPC's u32
	// id (the entity channel's, as the client knows it) and a u8 mark, 1 "!"
	// or 2 "?". An NPC not listed has none; the empty set clears them all.
	OpcodeQuestHeadMarks GameOpcode = 0x7E63
)
