package protocol

// A group's or a raid's ready check (the GM Jibrish's petition 1711, 2026-11-12;
// entity/readycheck.go, handlers/readycheck.go): /readycheck asks the group's other
// players "Are you ready?" for 30 seconds and tells the asker who is not. A client
// that shows the question in a window of its own (one that stops nothing: vanilla's)
// answers the offer and is sent the window; a PS2 ignores the offer (CLIENT's
// dispatcher has no case above 0x1402 and no default) and is told in a line to type
// "/t ready" or "/t notready".
const (
	// OpcodeReadyCheckOffer, server to client once the player is in the world: a u8, 1.
	OpcodeReadyCheckOffer GameOpcode = 0x7E49
	// OpcodeReadyCheckAnswer, client to server at once after the offer: a u8, 1 to be
	// sent the check's window (OpcodeReadyCheck), 0 for the line a PS2 gets.
	OpcodeReadyCheckAnswer GameOpcode = 0x7E4A
	// OpcodeReadyCheck, server to client to one that answered 1: u32 check id, u8
	// seconds left (0: the check is over for this client, close its window), the
	// asker's name (u8 byte count and its bytes; empty when closing).
	OpcodeReadyCheck GameOpcode = 0x7E4B
	// OpcodeReadyCheckReply, client to server: u32 check id, u8 1 ready, 0 not ready.
	OpcodeReadyCheckReply GameOpcode = 0x7E56
)
