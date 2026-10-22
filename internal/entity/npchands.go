package entity

import "github.com/eqoa/server/internal/database"

// npcHandTypes is the attack types an NPC's two hands carry on its channel
// (bytes 91 and 92, CLIENT's m_itemType), which the PS2 attaches the NPC's
// items with and picks its swing by. Until 2026-10-22 none was sent, and 0
// has no attack animation: an armed NPC swung without one.
//
// Sandstorm's are per NPC, not per weapon: one model is 1, 3 and 5 on three
// of its NPCs, and its smithies swing a hammer, a blunt 3 as an item, as a 1.
// Of the 33 NPC weapons its channels show in the PS2 captures, 27 are 1, and
// the catalog's type for the model would get fewer right than 1 does
// (claude-notes/held-items-channel-20261022.md). So a hand is: empty, 0;
// Sandstorm's own type where it showed this NPC holding this weapon
// (sandstormHands); the type of a model the catalog has only as a ranged
// weapon (ranged: the PS2 shoots a bow only as 7, a crossbow 9, a thrown
// weapon 10); else 1.
func npcHandTypes(name string, primary, secondary uint32, ranged map[uint32]byte) (byte, byte) {
	hand := func(model uint32) byte {
		if model == 0 {
			return 0
		}
		if t, ok := sandstormHands[npcHand{database.NPCClassKey(name), model}]; ok {
			return t
		}
		if t, ok := ranged[model]; ok {
			return t
		}
		return 1
	}
	return hand(primary), hand(secondary)
}

// npcHand is an NPC by its name (database.NPCClassKey) holding a weapon model.
type npcHand struct {
	name  string
	model uint32
}

// sandstormHands is every NPC weapon Sandstorm's channels show with a type
// other than npcHandTypes' rule gives (npchands_test.go holds the rule and
// these to all 33 it shows).
var sandstormHands = map[npcHand]byte{
	{"ambassador do'nerua", 0xBF7E7DCB}: 3,
	{"guard fitsroy", 0x024DB2A6}:       0,
	{"jasper darvar", 0x9461B864}:       5,
	{"merchant abirai", 0xBF7E7DCB}:     5,
	{"merchant akaron", 0x2D0FC238}:     5,
	{"merchant salania", 0x77F774ED}:    5,
}
