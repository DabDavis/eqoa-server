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

// ItemSlotsOffered is whether this server holds more than CLIENT in either list
// (its own or a grandfathered size), so that a client is offered them.
func ItemSlotsOffered() bool {
	pack, bank := ItemListsMost()
	return pack > ClientItemSlots || bank > ClientItemSlots
}

// Grandfathered lists (world.grandfather_pack_slots, world.grandfather_bank_slots;
// the server owner, 2026-11-18, of Vanilla Test: "limit bank and inventory slots
// back to 40 each", and the characters already past it keep what they hold).
//
// With a list grandfathered, pack_slots (bank_slots) is the most a NEW record may
// make: no item comes as a record of its own while the character holds that many
// or more (its list's rows and the rows kept beside it, handlers/itemlists.go). A
// character that held more when the limit came down keeps every one, its client
// sent them all in a list of max(limit, what it holds), up to the grandfathered
// size; and since nothing new comes while it holds the limit or more, the room it
// frees above the limit is gone for good: its allowance is max(limit, what it
// holds), and only ever shrinks. Nothing is stored for it: what the character
// holds is the allowance, read from its rows at every check (handlers/itemlist.go's
// roomBound), so it holds over restarts and keep restarts and needs no data step.
// Unset, or not above the limit: off, the list as without it.
var grandPack, grandBank atomic.Int32

// SetItemSlotsGrandfathered sets the grandfathered sizes (0: off), each at most
// MaxItemSlots: at the start, from config, after SetItemSlots.
func SetItemSlotsGrandfathered(pack, bank int) {
	clamp := func(n int) int32 {
		if n <= 0 {
			return 0
		}
		return int32(min(n, MaxItemSlots))
	}
	grandPack.Store(clamp(pack))
	grandBank.Store(clamp(bank))
}

// ItemSlotsGrandfathered is the grandfathered size of the pack and the bank,
// 0 for a list that is not (unset, or not above its limit, ItemSlots).
func ItemSlotsGrandfathered() (pack, bank int) { return ItemSlotsGrandfatheredFor(false) }

// ItemListsMost is the most records a client is offered of each list: the
// grandfathered size where there is one, else ItemSlots.
func ItemListsMost() (pack, bank int) { return ItemListsMostFor(false) }

// [OG] accounts' lists (world.og_pack_slots, world.og_bank_slots; the server
// owner, 2026-11-18, of Vanilla Test: "lets make 100 inventory space in bank and
// pack for vanilla test, but [OG] gets 40/40"). An [OG] account's session
// (handlers/ogaccount.go, world.og_accounts) holds at most these, every other
// session pack_slots and bank_slots. Below the shard's own size an [OG] list is
// grandfathered to it (as world.grandfather_pack_slots does for everyone): an
// [OG] character that holds more keeps every item and is sent them all, but no
// new record comes while it holds the [OG] size or more. Unset (0): an [OG]
// session's lists are everyone's.
var ogPack, ogBank atomic.Int32

// SetOGItemSlots sets the [OG] sizes (0: off, else clamped as ClampItemSlots):
// at the start, from config.
func SetOGItemSlots(pack, bank int) {
	clamp := func(n int) int32 {
		if n <= 0 {
			return 0
		}
		return int32(ClampItemSlots(n))
	}
	ogPack.Store(clamp(pack))
	ogBank.Store(clamp(bank))
}

// OGItemSlots is the [OG] sizes as set (0: off).
func OGItemSlots() (pack, bank int) { return int(ogPack.Load()), int(ogBank.Load()) }

// ItemSlotsFor is ItemSlots for a session of an [OG] account (og) or another:
// an [OG] one's at most its [OG] size.
func ItemSlotsFor(og bool) (pack, bank int) {
	pack, bank = ItemSlots()
	if og {
		if p := int(ogPack.Load()); p > 0 {
			pack = min(pack, p)
		}
		if b := int(ogBank.Load()); b > 0 {
			bank = min(bank, b)
		}
	}
	return pack, bank
}

// ItemSlotsGrandfatheredFor is ItemSlotsGrandfathered for an [OG] account's
// session (og) or another: an [OG] list below the shard's size is grandfathered
// to the larger of that size and the shard's grandfathered one.
func ItemSlotsGrandfatheredFor(og bool) (pack, bank int) {
	basePack, baseBank := ItemSlots()
	limPack, limBank := ItemSlotsFor(og)
	g := func(raw int32, base, lim int) int {
		n := int(raw)
		if lim < base {
			n = max(n, base)
		}
		if n > lim {
			return n
		}
		return 0
	}
	return g(grandPack.Load(), basePack, limPack), g(grandBank.Load(), baseBank, limBank)
}

// ItemListsMostFor is ItemListsMost for an [OG] account's session (og) or
// another; never more than ItemListsMost (what the offer named).
func ItemListsMostFor(og bool) (pack, bank int) {
	pack, bank = ItemSlotsFor(og)
	gPack, gBank := ItemSlotsGrandfatheredFor(og)
	return max(pack, gPack), max(bank, gBank)
}
