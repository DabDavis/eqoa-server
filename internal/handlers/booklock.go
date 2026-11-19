package handlers

import "sync"

// booklock.go: one character's spell book changed by one request at a time.
//
// Every packet is handled in a goroutine of its own (network/udp/turns.go), so
// two of one client's book requests can be handled at once. Each is a read of
// the book's rows, a change to them and a write of the rows it changed: two
// 0x0033s at once each read the book before the other's write, and each wrote
// its own ability onto the slot without taking the other's off. Petition 2146:
// a player's Bloodwraith (display index 63) and Shroud of Death + (74) both
// went onto bar slot 5 in the same instant (02:31:23, after a session-keeping
// restart resent them); the client, told 0x00B4 for each in order, had Shroud
// of Death + on the slot, the book both, and a press of the slot (0x0032 names
// the slot) cast the first of the book's: Bloodwraith, "Target is friendly."
// Likewise two abilities added at once (masteries, scrolls, quests) could take
// one display index. lockBook serializes them per character.

var bookLocks sync.Map // serverid -> *sync.Mutex

// lockBook locks character serverID's spell book and returns its unlock. It
// is a leaf lock: nothing that takes another lock is called while it is held.
func lockBook(serverID uint32) func() {
	m, _ := bookLocks.LoadOrStore(serverID, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
