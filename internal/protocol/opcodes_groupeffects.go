package protocol

// The effects on each of the player's group members, for the tiny icons a client draws
// beside their bars (entity/groupeffects.go, handlers/groupeffects.go; jibrish,
// 2026-11-22: "add very small / tiny icon versions for the buffs group members have").
// CLIENT's group bars (VIWndGame::DrawGroup) show a member's name, health and two status
// icons, and its effects window is channel 0x43, the player's own: no message of the game's
// carries another's effects. Not retail, away from every lane's range (0x7EB0-0x7EB2 the
// Hardcore characters'). A PS2 ignores the offer (CLIENT's dispatcher has no case above
// 0x1402 and no default) and is sent none.
const (
	// OpcodeGroupEffects, both ways, a u32: the server offers it (1) once the player is in
	// the world; a client that draws its members' effects answers 1 to be sent the set
	// (OpcodeGroupEffectsData) and 0 to stop, at once and whenever that changes.
	OpcodeGroupEffects GameOpcode = 0x7EC0
	// OpcodeGroupEffectsData, server to a client that answered 1, at once and whenever the
	// set changes: a u8 count of members, then each member's u32 id (as the group's roster
	// names it to that client: a player's or a merc's, groupFarID's stand-in out of sight),
	// a u8 count of its effects and each effect's u32 icon (a DictID, as channel 0x43's), in
	// the order they went on: what the member's own buff window shows of it (at most 16).
	// Each is the whole set: a member it leaves out has no icons.
	OpcodeGroupEffectsData GameOpcode = 0x7EC1
)
