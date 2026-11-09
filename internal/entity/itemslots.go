package entity

import "sync/atomic"

// itemslots.go — how many records the pack and the bank hold (world.pack_slots,
// world.bank_slots; the server owner, 2026-11-09: "lets increase inventory and bank
// space to 100"). handlers/itemlists.go has the offer, the lists and what a client
// that holds fewer is sent.
//
// CLIENT holds 40 of each and no more: VIItemList is `int ItemCount; int m_p04;
// VINetItem ItemInternal[40]` (decomp game.h), one class for the pack (VIClient
// +0xC428, worn items among its records), the bank (VIBankControl +8), the corpse,
// the merchant and the trade's copy; every AddItem is called with 40, which keeps
// nothing past it, and VIItemList::Read (the MemoryDump's lists) writes as many
// records as the count says into those 40 with no check. So a PS2, and any client
// that has not said it holds more, is never sent more than 40 in either list.

// ClientItemSlots is CLIENT's: the records each item list holds.
const ClientItemSlots = 40

// MaxItemSlots is the most either list may be set to: the server names a record
// by its list index in a byte (0x003C, 0x003E, 0x003F, 0x1250).
const MaxItemSlots = 255

var packSlots, bankSlots atomic.Int32

func init() { SetItemSlots(ClientItemSlots, ClientItemSlots) }

// ClampItemSlots is n as a list's size: ClientItemSlots for 0 or less, never
// past MaxItemSlots, never below ClientItemSlots.
func ClampItemSlots(n int) int {
	return max(ClientItemSlots, min(n, MaxItemSlots))
}

// SetItemSlots sets the pack's and the bank's records (world.pack_slots and
// world.bank_slots), each clamped: at the start, from config.
func SetItemSlots(pack, bank int) {
	packSlots.Store(int32(ClampItemSlots(pack)))
	bankSlots.Store(int32(ClampItemSlots(bank)))
}

// ItemSlots is how many records the pack and the bank hold on this server, for
// a client that holds them (40 each: CLIENT's, nothing offered).
func ItemSlots() (pack, bank int) {
	return int(packSlots.Load()), int(bankSlots.Load())
}

// ItemSlotsOffered is whether this server holds more than CLIENT in either list,
// so that a client is offered them.
func ItemSlotsOffered() bool {
	pack, bank := ItemSlots()
	return pack > ClientItemSlots || bank > ClientItemSlots
}
