package udp

import (
	"net/netip"
	"time"
)

// turns.go: the packets of one address in the order the read loop read them.
//
// Every packet is handled in a goroutine of its own (readLoop), so two of one
// client's may be handled at once, the later one first. Only putting a long
// message back together waits on that (session/fragments.go): it must know
// whether the message before one was a piece, and when that message's packet
// is still being handled, Assemble is unsure. The packet then waits for the
// packets read before it from its address (waitEarlier), at most turnWait,
// and asks again. Nothing else waits: every other packet is handled as it was.

// turnWait is the longest a packet waits for those read before it: past it, a
// packet stuck in its handler holds up no other.
const turnWait = time.Second

// packetTurn is one packet's place among its address's packets.
type packetTurn struct {
	// earlier is closed once every packet read before this one from its
	// address is handled (nil: none was)
	earlier <-chan struct{}
	// done is closed once this one is handled, and those before it
	done chan struct{}
}

// waitEarlier returns once the packets read before t from its address are
// handled, or turnWait has passed.
func (t *packetTurn) waitEarlier() {
	if t == nil || t.earlier == nil {
		return
	}
	select {
	case <-t.earlier:
		return
	default:
	}
	timer := time.NewTimer(turnWait)
	defer timer.Stop()
	select {
	case <-t.earlier:
	case <-timer.C:
	}
}

// finish says t is handled, once those before it are (at most turnWait more).
func (t *packetTurn) finish() {
	if t == nil {
		return
	}
	t.waitEarlier()
	close(t.done)
}

// nextTurn links a packet to the latest in-flight packet from its address.
func (l *Listener) nextTurn(addr netip.AddrPort) *packetTurn {
	l.turnMu.Lock()
	defer l.turnMu.Unlock()
	if l.turns == nil {
		l.turns = make(map[netip.AddrPort]chan struct{})
	}
	t := &packetTurn{earlier: l.turns[addr], done: make(chan struct{})}
	l.turns[addr] = t.done
	return t
}

// finishTurn removes the last turn once its chain is done. A newer packet may
// already have installed its own turn while this one was being handled.
func (l *Listener) finishTurn(addr netip.AddrPort, t *packetTurn) {
	t.finish()
	l.turnMu.Lock()
	if l.turns[addr] == t.done {
		delete(l.turns, addr)
	}
	l.turnMu.Unlock()
}
