package session

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// fragments.go: a reliable message a client sent in pieces, put back together
// before it is handled, as DRDP's own receive does.
//
// connection_send_guaranteed (decomp src/connection.cpp; vanilla's drdp
// conn.rs) cuts a reliable message longer than 1,300 bytes into pieces of
// 1,300, each under a sequence number of its own: every piece but the last is
// typed 0xFA (MESSAGE_TYPE_FRAGMENT), the last keeps the message's own type
// (0xFB, or 0xF9 for a system message), and only the first starts with the
// opcode. connection_move_delayed_to_arrivals holds the pieces until the last
// one lands and hands on one message. Until 2026-11-08 this server took every
// piece for a message of its own: the first went to its handler cut short (a
// petition past 645 characters was refused), each later one to whatever
// handler its first two bytes named.
//
// A client message that long is vanilla's (a long petition; a PS2's longest,
// a petition or a gram of 511 characters, is under 1,100 bytes), and a client
// sends one piece a packet, about 360 ms apart (its 3,000 bytes a second,
// bandwidth.cpp). The server takes each reliable message as its packet comes,
// a goroutine a packet, and acknowledges the newest number it has seen, so a
// message lost before a later one arrived is never sent again. So the pieces
// are put back together only when they are known to be one message: a run
// starts with a piece whose number follows a message known not to be a piece,
// goes on number by number, and ends with the next 0xFB or 0xF9. Where the
// message before one is not known yet (its packet is still being handled),
// Assemble says so and the listener asks again once the packets read before
// this one from the same address are handled (udp/turns.go). A gap, a run past
// FragmentedMost or one not finished within fragmentRunFor drops the whole
// message, logged; no piece of it is ever handed on.

const (
	// FragmentedMost is the most bytes a message put back together may hold,
	// its opcode with them (a petition of 2,047 characters is 4,103).
	FragmentedMost = 16 << 10
	// fragmentRunFor is how long a message's pieces may take: one of a dozen
	// pieces comes in about four seconds.
	fragmentRunFor = time.Minute
	// rxKindsKept is how many of the client's last reliable numbers are known
	// by kind (a piece or not).
	rxKindsKept = 1024
)

// Assembly is what Assemble makes of a reliable message.
type Assembly int

const (
	// AssemblyDeliver: a message whole: handle it as it is.
	AssemblyDeliver Assembly = iota
	// AssemblyWhole: the last piece of a message: handle the message
	// returned, its pieces put back together.
	AssemblyWhole
	// AssemblyHeld: a piece, kept until the last one comes.
	AssemblyHeld
	// AssemblyDropped: part of a message that cannot be put back together,
	// or a message too short to hold an opcode: nothing to handle.
	AssemblyDropped
	// AssemblyUnsure: whether the message before it was a piece is not known
	// yet. Nothing is kept: ask again, sure, once the packets read before this
	// one are handled.
	AssemblyUnsure
)

// what the message before a message was
const (
	prevUnknown = iota
	prevWhole   // a message whole, or the last piece of one
	prevPiece   // a piece of a message not being put back together
	prevRun     // the last piece the open run took
)

type rxKind struct {
	seq        uint16
	set, piece bool
}

// fragRun is a message being put back together, or dropped (data nil).
type fragRun struct {
	first, next uint16 // its first piece's number, and its next piece's
	opcode      uint16 // its first piece's first two bytes, for the log
	data        []byte
	at          time.Time
}

type assembler struct {
	kinds [rxKindsKept]rxKind
	run   *fragRun
	// said is what take has to log, for Assemble to log
	said []string
}

// Assemble takes the client's reliable message m (0xFA, 0xFB or 0xF9, a copy
// of one already taken left out), received at now, and says what to handle:
// m itself, the message it completes, or nothing. Unless sure, a message
// whose predecessor is not known yet comes back AssemblyUnsure, with nothing
// recorded; sure, it is decided on what is known.
func (cd *ConnectionData) Assemble(m *protocol.Message, sure bool, now time.Time) (Assembly, *protocol.Message) {
	cd.mu.Lock()
	v, whole := cd.frags.take(m, sure, now)
	said := cd.frags.said
	cd.frags.said = nil
	cd.mu.Unlock()
	// logged with no lock held (the address is the session's, under its own)
	for _, line := range said {
		log.Printf("Fragments: %s's %s", cd.who(), line)
	}
	return v, whole
}

// who is the connection's client, for a log line.
func (cd *ConnectionData) who() string {
	if cd.session != nil {
		return cd.session.RemoteAddr().String()
	}
	return "a client"
}

func (a *assembler) note(seq uint16, piece bool) {
	a.kinds[seq%rxKindsKept] = rxKind{seq: seq, set: true, piece: piece}
}

func (a *assembler) prev(seq uint16) int {
	if a.run != nil && a.run.next == seq {
		return prevRun
	}
	p := seq - 1
	if k := a.kinds[p%rxKindsKept]; k.set && k.seq == p {
		if k.piece {
			return prevPiece
		}
		return prevWhole
	}
	if p == 0 {
		return prevWhole // a connection numbers its first message 1
	}
	return prevUnknown
}

// drop says why the open run's message is dropped, if one is being put
// together.
func (a *assembler) drop(why string) {
	a.dropRun(a.run, why)
}

func (a *assembler) dropRun(r *fragRun, why string) {
	if r != nil && r.data != nil {
		a.said = append(a.said, fmt.Sprintf("message 0x%04X (%d bytes in pieces %d-%d) dropped: %s",
			r.opcode, len(r.data), r.first, r.next-1, why))
	}
}

func (a *assembler) take(m *protocol.Message, sure bool, now time.Time) (Assembly, *protocol.Message) {
	seq := m.Sequence
	if a.run != nil && now.Sub(a.run.at) > fragmentRunFor {
		a.drop("its last piece never came")
		a.run = nil
	}
	prev := a.prev(seq)
	if prev == prevUnknown && !sure {
		return AssemblyUnsure, nil
	}
	data := m.Data()
	if m.Type == protocol.MessageTypeSegmentReliable {
		a.note(seq, true)
		switch prev {
		case prevRun:
			r := a.run
			r.next = seq + 1
			if r.data == nil {
				return AssemblyDropped, nil
			}
			if len(r.data)+len(data) > FragmentedMost {
				a.drop("longer than the server takes")
				r.data = nil
				return AssemblyDropped, nil
			}
			r.data = append(r.data, data...)
			return AssemblyHeld, nil
		case prevWhole:
			a.drop("a piece of it never came")
			r := &fragRun{first: seq, next: seq + 1, data: append([]byte(nil), data...), at: now}
			if len(data) >= 2 {
				r.opcode = uint16(data[0]) | uint16(data[1])<<8
			}
			a.run = r
			return AssemblyHeld, nil
		}
		// the message before it was a piece of another, or is not known: a
		// piece whose message's start is not here
		if a.run == nil {
			a.said = append(a.said, fmt.Sprintf("piece %d dropped: its message's start is not here", seq))
		}
		a.drop("a piece of it never came")
		a.run = &fragRun{first: seq, next: seq + 1, at: now}
		return AssemblyDropped, nil
	}

	// a message whole, or the last piece of one (0xFB, 0xF9)
	a.note(seq, false)
	switch prev {
	case prevRun:
		r := a.run
		a.run = nil
		if r.data == nil {
			return AssemblyDropped, nil
		}
		wire := m.Wire()
		if len(r.data)+len(wire) > FragmentedMost {
			a.dropRun(r, "longer than the server takes")
			return AssemblyDropped, nil
		}
		whole := append(r.data, wire...)
		out := protocol.GetMessage()
		out.Type = m.Type
		out.Sequence = r.first
		out.Created, out.LastSent = now, now
		if m.Type == protocol.MessageTypeReliable && len(whole) >= 2 {
			out.Opcode = protocol.GameOpcode(uint16(whole[0]) | uint16(whole[1])<<8)
			out.SetData(whole[2:])
		} else {
			out.Unsplit = true
			out.SetData(whole)
		}
		return AssemblyWhole, out
	case prevPiece:
		// the last piece of a message whose pieces are not all here
		a.drop("a piece of it never came")
		a.run = nil
		return AssemblyDropped, nil
	case prevUnknown:
		if a.run != nil {
			// pieces missing between the open run and this: its last piece
			a.drop("a piece of it never came")
			a.run = nil
			return AssemblyDropped, nil
		}
	default:
		if a.run != nil {
			a.drop("its next piece never came")
			a.run = nil
		}
	}
	if m.Unsplit && m.Type == protocol.MessageTypeReliable {
		return AssemblyDropped, nil // too short for an opcode
	}
	return AssemblyDeliver, nil
}

// KeptFragments is a message being put back together as a restart carries it
// (keep.go): the open run's numbers and what it holds (nil: being dropped).
type KeptFragments struct {
	First  uint16 `json:"first"`
	Next   uint16 `json:"next"`
	Opcode uint16 `json:"opcode,omitempty"`
	Data   []byte `json:"data,omitempty"`
}

// keepLocked is the open run, nil with none. cd.mu is held.
func (a *assembler) keepLocked() *KeptFragments {
	r := a.run
	if r == nil {
		return nil
	}
	return &KeptFragments{First: r.first, Next: r.next, Opcode: r.opcode, Data: append([]byte(nil), r.data...)}
}

// restoreLocked puts a kept run back, and the newest message the old process
// took (rx) as a piece of it or not. cd.mu is held.
func (a *assembler) restoreLocked(k *KeptFragments, rx uint16, now time.Time) {
	*a = assembler{}
	if k != nil {
		a.run = &fragRun{first: k.First, next: k.Next, opcode: k.Opcode, at: now}
		if len(k.Data) > 0 {
			a.run.data = append([]byte(nil), k.Data...)
		}
	}
	a.note(rx, a.run != nil && a.run.next == rx+1)
}
