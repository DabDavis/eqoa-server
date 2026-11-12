package protocol

// The buff window (the GM Jibrish's petition 1761, 2026-11-12; entity/buffwindow.go,
// handlers/buffwindow.go): the player's effects in two rows and its pet's in a third, each
// with its name and time left, a beneficial one taken off by a click or a controller's
// press. A client that draws such a window answers the offer and is sent the set; a PS2
// ignores the offer (CLIENT's dispatcher has no case above 0x1402 and no default) and keeps
// channel 0x43's window, as it was.
const (
	// OpcodeBuffWindow, both ways, a u8: the server offers it (1) once the player is in the
	// world; a client that draws the window answers 1 to be sent the set
	// (OpcodeBuffWindowData) and 0 to stop, at once and whenever that changes.
	OpcodeBuffWindow GameOpcode = 0x7E57
	// OpcodeBuffWindowData, server to a client that answered 1, at once and whenever the set
	// changes: a u8 count, then each line's u8 whose (0 the player, 1 its pet), u8 flags
	// (bit 0: the player may take it off), u32 id (the effect's spell id; 0 for a channel
	// 0x43 slot no effect backs), u32 icon (a DictID, as channel 0x43's), u32 milliseconds
	// left as the message is made (0xFFFFFFFF: no end kept) and its name (a u8 byte count
	// and UTF-8, at most 31 characters). The player's lines first, in the order they went
	// on (at most 16), then the pet's (at most 8). Each is the whole set.
	OpcodeBuffWindowData GameOpcode = 0x7E58
	// OpcodeBuffRemove, client to server: u8 whose, u32 id, u32 icon, a line of the set to
	// take off. The server takes off a beneficial one, tells the player in a line either
	// way and sends the set again; anything else is refused in a line.
	OpcodeBuffRemove GameOpcode = 0x7E59
)
