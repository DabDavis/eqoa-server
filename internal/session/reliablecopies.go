package session

import (
	"hash/fnv"
	"time"
)

// reliablecopies.go: a client's reliable message taken once (petition 1032,
// Jibrish: "Sometimes if you send a message in chat it gets duplicated
// repeatedly ... despite only hitting enter once").
//
// A DRDP client resends every reliable message it has not seen acknowledged
// in each packet it sends until the acknowledgment comes (CLIENT's
// connection.cpp; the server does the same), so a stalled link (a relay's or
// a phone's: the client's packets held, then delivered at once) brings the
// same message many times: Zaeire's shout of 03:26:51 UTC 2026-11-06 went to
// eight players 24 times in that second. CLIENT keeps its own dedup of the
// server's messages (connection_has_reliable: at or before its rx, or held
// out of order), and so does the server now, the same way: by number, in the
// window of rxwindow.go (at or before the ack, or taken past it), on every
// session, resumed through a restart or not. From 2026-11-06 to 2026-11-09 a
// copy was known by its number AND contents within 30 s, since the client's
// lobby connection, after a promotion, could resend 0x002A numbered in its
// own space and a window by number would have taken it for the newest; a
// promoted session now leaves the lobby connection's packets alone
// (PromotedOwn), and a check by contents let a copy through 30 s on.

// TakeReliableOnce records the client's reliable message seq and reports
// whether it is new: false for a copy (at or before the ack, or taken past
// it), which is acknowledged again but not handled a second time, and for one
// beyond the window (handled when it comes again, once the ack has moved).
// sum and now are no longer read (the window alone decides).
func (cd *ConnectionData) TakeReliableOnce(seq uint16, sum uint64, now time.Time) bool {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.takeLocked(seq)
}

// ReliableSum is what a reliable message's copy was known by beside its
// number: its type, opcode and bytes (TakeReliableOnce no longer reads it).
func ReliableSum(typ byte, opcode uint16, data []byte) uint64 {
	h := fnv.New64a()
	h.Write([]byte{typ, byte(opcode), byte(opcode >> 8)})
	h.Write(data)
	return h.Sum64()
}
