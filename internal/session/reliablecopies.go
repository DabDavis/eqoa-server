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
// connection.cpp; the server does the same). The server took every reliable
// message it received on a session that had not come through a restart, so a
// stalled link (a relay's or a phone's: the client's packets held, then
// delivered at once) had it handle each copy: Zaeire's shout of 03:26:51
// UTC 2026-11-06 went to eight players 24 times in that second, every copy
// in a packet newer than the last (the older ones were dropped as late).
// CLIENT keeps its own dedup of the server's messages (drdp-decompiled.md:
// the arrival bitmap at 0x1158), so the server's own resends were never
// shown twice.
//
// A copy is known by its number AND its contents: the same sequence number,
// type, opcode and bytes as a message taken within reliableTakenFor. Not by
// the number alone as TakeReliable does on a resumed session: after a
// promotion the client's old lobby connection can resend 0x002A with a number
// of its own space, and a window by number would then take that number as
// the newest and drop the world's real messages below it as old. A message
// whose number and contents both match one just taken can only be a copy.
// The entries are forgotten at the master session's reset with the numbers
// (ResetForMasterSession), so a new connection's first messages, which may
// be byte for byte the lobby's, are never taken for copies.

// reliableTakenKept is how many of the client's last reliable messages are
// kept to know their copies by (a client resends nothing older than its
// window of unacknowledged messages, far fewer than this), and
// reliableTakenFor how long one is (a resend of a message unanswered that
// long is a new attempt, which the handler may answer again).
const (
	reliableTakenKept = 256
	reliableTakenFor  = 30 * time.Second
)

type reliableTaken struct {
	seq uint16
	sum uint64
	at  time.Time
}

// ReliableSum is what a reliable message's copy is known by beside its
// number: its type, opcode and bytes.
func ReliableSum(typ byte, opcode uint16, data []byte) uint64 {
	h := fnv.New64a()
	h.Write([]byte{typ, byte(opcode), byte(opcode >> 8)})
	h.Write(data)
	return h.Sum64()
}

// TakeReliableOnce records the client's reliable message seq, of contents
// sum, received at now, and reports whether it is new: false for a copy of
// one taken within reliableTakenFor (the same number and contents), which is
// acknowledged again but not handled a second time. 0 is never recorded.
func (cd *ConnectionData) TakeReliableOnce(seq uint16, sum uint64, now time.Time) bool {
	if seq == 0 {
		return true
	}
	cd.mu.Lock()
	defer cd.mu.Unlock()
	for i := range cd.rxTaken {
		t := &cd.rxTaken[i]
		if !t.at.IsZero() && t.seq == seq && t.sum == sum && now.Sub(t.at) < reliableTakenFor {
			return false
		}
	}
	cd.rxTaken[cd.rxNext] = reliableTaken{seq: seq, sum: sum, at: now}
	cd.rxNext = (cd.rxNext + 1) % reliableTakenKept
	if SeqAfter(seq, cd.lastReceivedMessageSeq) {
		cd.lastReceivedMessageSeq = seq
	}
	return true
}
