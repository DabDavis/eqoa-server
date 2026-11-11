package entity

import "sync/atomic"

// charslots.go — how many characters an account may hold (world.max_characters;
// the server owner, 2026-11-11: "lets let player create more than 8 chars, also
// allow client to scroll for the additional slots. lets make max 20 for now").
// handlers/charslots.go has what each client is sent and may create.
//
// CLIENT's select page holds 8: VIClient::CharacterList reads the count, keeps it
// at 8 at most (`m_charCount > 8 ? 8`) and reads that many records into
// `VICharRecord m_chars[8]` (decomp viclient.cpp, game.h +0x2BE70), the rest of
// the message unread. A longer list does not overrun it, but it shows the first
// 8 the server sent and offers no empty slot to create in. So a PS2, and any
// client that has not said its page lists more, is sent 8 at most (the most
// recently played) and may not create a ninth.

// ClientCharSlots is CLIENT's: the characters its select page holds.
const ClientCharSlots = 8

// DefaultCharSlots is the characters an account may hold where
// world.max_characters is unset.
const DefaultCharSlots = 20

// MaxCharSlots is the most world.max_characters may be set to.
const MaxCharSlots = 64

var charSlots atomic.Int32

func init() { SetCharSlots(0) }

// ClampCharSlots is n as an account's characters: DefaultCharSlots for 0 or
// less, never past MaxCharSlots.
func ClampCharSlots(n int) int {
	if n <= 0 {
		return DefaultCharSlots
	}
	return min(n, MaxCharSlots)
}

// SetCharSlots sets the characters an account may hold (world.max_characters),
// clamped: at the start, from config.
func SetCharSlots(n int) { charSlots.Store(int32(ClampCharSlots(n))) }

// CharSlots is the characters an account may hold on this server, made and
// listed by a client whose select page lists them all (vanilla).
func CharSlots() int { return int(charSlots.Load()) }

// ClientCharCap is the characters a client may be listed and may make: this
// server's own for one whose page lists them all (many), else CLIENT's 8 (or
// the server's, where it is fewer).
func ClientCharCap(many bool) int {
	if many {
		return CharSlots()
	}
	return min(ClientCharSlots, CharSlots())
}
