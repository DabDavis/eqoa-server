package entity

import "encoding/binary"

// hidelook.go — a worn item's look hidden (the owner, 2026-11-11: "lets add a
// hide display option to equipped armor/weapon, still equipped, but no model
// displayed"). A character marks equip slots hidden (Player.HiddenLocs, table
// charhiddenlooks, from a client offered it: handlers/hidelook.go); whatever
// is worn in one keeps its stats, its effects and its swing, and only what
// the clients are sent of it is bare, as a Wisp in its own form is sent
// (wispform.go): the entity channel's held item, armour set and its tint or
// robe (hideLookParts), the character list's gear (HideCharacterParts) and
// the cape (handlers.wornLook leaves it unworn). A held item's attack type
// stays, so the hand swings as its weapon's with nothing in it. Every client
// sees it so, the player's own too (CLIENT dresses its own sprite off its
// channel), a PS2 as well; the ctf banner (an override item) stays.

// The parts of the look a hidden slot hides (Player.HiddenParts).
const (
	HideChest uint16 = 1 << iota
	HideBracer
	HideGloves
	HideLegs
	HideBoots
	HideHelm
	HideRobe
	HidePrimary // the channel's item 0, the main hand
	HideSecondary
	HideShield
	HideCape
)

// HideLookSlots is the equip slots a character may hide: CLIENT's 0-21 and
// the cape's 22 (handlers' equipCape).
const HideLookSlots = 23

// hideLookParts makes an entity channel buffer (the disc's 162 bytes) show the
// hidden parts as nothing worn: no held item (its attack type kept), no armour
// set and no tint, no robe.
func hideLookParts(buf []byte, parts uint16) {
	if parts == 0 {
		return
	}
	for i, h := range []uint16{HidePrimary, HideSecondary, HideShield} {
		if parts&h != 0 {
			binary.LittleEndian.PutUint32(buf[67+4*i:], 0)
		}
	}
	for i, h := range []uint16{HideChest, HideBracer, HideGloves, HideLegs, HideBoots, HideHelm} {
		if parts&h != 0 {
			buf[93+i], buf[99+i] = 0, 0
		}
	}
	if parts&HideRobe != 0 {
		buf[109], buf[110] = 0xFF, 0
	}
}

// HideCharacterParts makes a 0x002C record's 32 gear bytes (handlers'
// characterGear) show the hidden parts as the list sends a bare slot: no robe
// (0xFFFFFFFF, tint 0), no held item (its attack type kept), armour 0xFF and
// tint 0.
func HideCharacterParts(g []byte, parts uint16) {
	if len(g) < 32 || parts == 0 {
		return
	}
	if parts&HideRobe != 0 {
		binary.LittleEndian.PutUint32(g[0:4], 0xFFFFFFFF)
		g[4] = 0
	}
	for i, h := range []uint16{HidePrimary, HideSecondary, HideShield} {
		if parts&h != 0 {
			binary.LittleEndian.PutUint32(g[5+4*i:], 0)
		}
	}
	for i, h := range []uint16{HideChest, HideBracer, HideGloves, HideLegs, HideBoots, HideHelm} {
		if parts&h != 0 {
			g[20+i], g[26+i] = 0xFF, 0
		}
	}
}
