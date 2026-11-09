package session

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// pieces is message whole (its opcode first) as connection_send_guaranteed
// sends it from number seq, read as the parser reads each: 0xFA pieces of
// 1,300 bytes whole, then the last as a 0xFB whose opcode is split off when it
// has two bytes.
func pieces(seq uint16, whole []byte) []*protocol.Message {
	var out []*protocol.Message
	for off := 0; off == 0 || off < len(whole); off += 1300 {
		end := min(off+1300, len(whole))
		m := &protocol.Message{Type: protocol.MessageTypeSegmentReliable, Sequence: seq}
		if end < len(whole) {
			m.Unsplit = true
			m.SetData(whole[off:end])
		} else {
			m = wholeMsg(seq, whole[off:end])
		}
		out = append(out, m)
		seq++
	}
	return out
}

// wholeMsg is a 0xFB of these bytes as the parser reads it.
func wholeMsg(seq uint16, wire []byte) *protocol.Message {
	m := &protocol.Message{Type: protocol.MessageTypeReliable, Sequence: seq}
	if len(wire) >= 2 {
		m.Opcode = protocol.GameOpcode(binary.LittleEndian.Uint16(wire))
		m.SetData(wire[2:])
	} else {
		m.Unsplit = true
		m.SetData(wire)
	}
	return m
}

// petition is a request of n characters as vanilla writes one: 0x0AA1, sub-op
// 0, Bug Report (zigzag 1), Quest (zigzag 12), a u32 count, the units.
func petition(n int) []byte {
	b := []byte{0xA1, 0x0A, 0, 2, 24}
	b = binary.LittleEndian.AppendUint32(b, uint32(n))
	for i := 0; i < n; i++ {
		b = binary.LittleEndian.AppendUint16(b, uint16('a'+i%26))
	}
	return b
}

type step struct {
	v Assembly
	m *protocol.Message
}

// feed takes each message as the listener does: unsure, then sure.
func feed(cd *ConnectionData, now time.Time, ms ...*protocol.Message) []step {
	var out []step
	for _, m := range ms {
		v, w := cd.Assemble(m, false, now)
		if v == AssemblyUnsure {
			v, w = cd.Assemble(m, true, now)
		}
		out = append(out, step{v, w})
	}
	return out
}

// handled is what the listener hands to the handlers: each message delivered
// as it is or put back together, as its opcode and data.
func handled(t *testing.T, in []*protocol.Message, steps []step) [][]byte {
	t.Helper()
	var out [][]byte
	for i, s := range steps {
		switch s.v {
		case AssemblyDeliver:
			out = append(out, in[i].Wire())
		case AssemblyWhole:
			out = append(out, s.m.Wire())
		case AssemblyUnsure:
			t.Fatalf("message %d: unsure after the sure call", i)
		}
	}
	return out
}

// A petition of 2,047 characters (4,103 bytes) comes as three pieces and a
// last one and is handed on whole, once, as its opcode and data, between the
// messages whole on either side, which go as they came. The control: a
// petition of 511 (a PS2's longest, 1,031 bytes) is one message and goes as
// it came.
func TestAMessageInPiecesIsHandledWhole(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	now := time.Now()
	long := petition(2047)
	in := []*protocol.Message{wholeMsg(1, []byte{0x0B, 0x00, 'h', 'i'})}
	in = append(in, pieces(2, long)...)
	in = append(in, wholeMsg(6, petition(511)))
	if len(in) != 6 || in[1].Type != protocol.MessageTypeSegmentReliable || in[4].Type != protocol.MessageTypeReliable {
		t.Fatalf("the petition did not cut into 3 pieces and a last: %d messages", len(in))
	}
	steps := feed(cd, now, in...)
	for i, want := range []Assembly{AssemblyDeliver, AssemblyHeld, AssemblyHeld, AssemblyHeld, AssemblyWhole, AssemblyDeliver} {
		if steps[i].v != want {
			t.Fatalf("message %d: %v, want %v", i, steps[i].v, want)
		}
	}
	got := handled(t, in, steps)
	if len(got) != 3 || !bytes.Equal(got[1], long) || !bytes.Equal(got[2], petition(511)) {
		t.Fatalf("handed on %d messages; the long one %d bytes of %d", len(got), len(got[1]), len(long))
	}
	w := steps[4].m
	if w.Opcode != 0x0AA1 || w.Type != protocol.MessageTypeReliable || w.Sequence != 2 || w.Unsplit {
		t.Fatalf("put back together: opcode %#x type %#x seq %d unsplit %v", uint16(w.Opcode), w.Type, w.Sequence, w.Unsplit)
	}
}

// The last piece of a message of 1,301 bytes is one byte, read whole, and
// completes it; a message whole that short has no opcode and is not handed on.
func TestAOneBytePiece(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	msg := petition(646) // 9 + 1,292 = 1,301 bytes
	in := pieces(1, msg)
	if len(in) != 2 || len(in[1].Wire()) != 1 || !in[1].Unsplit {
		t.Fatalf("not a piece and one byte: %d messages", len(in))
	}
	in = append(in, wholeMsg(3, []byte{7}))
	got := handled(t, in, feed(cd, time.Now(), in...))
	if len(got) != 1 || !bytes.Equal(got[0], msg) {
		t.Fatalf("handed on %d messages", len(got))
	}
}

// A piece lost (the server acknowledges the newest number it saw, so the
// client never sends it again) drops the whole message: no piece of it reaches
// a handler, its last piece included, and the next message whole goes on. A
// piece whose message's start never came is dropped with the rest of it.
func TestAGapDropsTheWholeMessage(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	in := pieces(1, petition(2047)) // 1, 2, 3 pieces; 4 the last
	lost := []*protocol.Message{in[0], in[2], in[3], wholeMsg(5, []byte{0x0B, 0x00, 'o', 'k'})}
	got := handled(t, lost, feed(cd, time.Now(), lost...))
	if len(got) != 1 || string(got[0]) != "\x0b\x00ok" {
		t.Fatalf("handed on %q", got)
	}

	// the first piece lost: the rest has no start
	cd = NewConnectionData(nil, 0, 0)
	in = pieces(10, petition(2047))
	nostart := append([]*protocol.Message{wholeMsg(8, []byte{0x0B, 0x00})}, in[1:]...) // 10 lost
	got = handled(t, nostart, feed(cd, time.Now(), nostart...))
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("a message with no start: handed on %d messages", len(got))
	}
	// the control: the same pieces after the message before them, whole
	cd = NewConnectionData(nil, 0, 0)
	whole := append([]*protocol.Message{wholeMsg(9, []byte{0x0B, 0x00})}, in...)
	if got := handled(t, whole, feed(cd, time.Now(), whole...)); len(got) != 2 {
		t.Fatalf("the control: handed on %d messages", len(got))
	}
}

// Two packets handled at once, the later first (a goroutine each): a message
// whose predecessor is not known yet is unsure and nothing is recorded of it;
// asked again once the earlier packet is in, it is decided right. Here the last
// piece of a two-piece message comes before the first.
func TestTheLastPieceBeforeItsPredecessor(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	now := time.Now()
	msg := petition(700)
	in := pieces(5, msg)
	if got := feed(cd, now, wholeMsg(4, []byte{0x0B, 0x00})); got[0].v != AssemblyDeliver {
		t.Fatalf("message 4: %v", got[0].v)
	}
	// the last piece's packet first
	if v, _ := cd.Assemble(in[1], false, now); v != AssemblyUnsure {
		t.Fatalf("the last piece, its predecessor unknown: %v", v)
	}
	// the first piece's packet, then the last piece asked again
	if v, _ := cd.Assemble(in[0], false, now); v != AssemblyHeld {
		t.Fatalf("the first piece: %v", v)
	}
	v, w := cd.Assemble(in[1], true, now)
	if v != AssemblyWhole || !bytes.Equal(w.Wire(), msg) {
		t.Fatalf("the last piece asked again: %v", v)
	}
	// a message whole whose predecessor is lost: unsure, then sure, handed on
	// (as every message was)
	if v, _ := cd.Assemble(wholeMsg(9, []byte{0x0B, 0x00}), false, now); v != AssemblyUnsure {
		t.Fatalf("message 9 after a gap: %v", v)
	}
	if v, _ := cd.Assemble(wholeMsg(9, []byte{0x0B, 0x00}), true, now); v != AssemblyDeliver {
		t.Fatalf("message 9, sure: %v", v)
	}
}

// What the server will not hold: a message past FragmentedMost, a run not
// finished within fragmentRunFor. Each is dropped whole, and what follows goes
// on. A system message (0xF9) may end a run and comes back one, its bytes
// whole.
func TestRunsTheServerWillNotHold(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	now := time.Now()
	big := make([]byte, FragmentedMost+1)
	big[0], big[1] = 0xA1, 0x0A
	in := append(pieces(1, big), wholeMsg(uint16(len(pieces(1, big))+1), []byte{0x0B, 0x00}))
	if got := handled(t, in, feed(cd, now, in...)); len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("a message past FragmentedMost: handed on %d", len(got))
	}
	// the most it takes, the control
	cd = NewConnectionData(nil, 0, 0)
	most := big[:FragmentedMost]
	in = pieces(1, most)
	if got := handled(t, in, feed(cd, now, in...)); len(got) != 1 || !bytes.Equal(got[0], most) {
		t.Fatalf("a message of FragmentedMost: handed on %d", len(got))
	}

	cd = NewConnectionData(nil, 0, 0)
	in = pieces(1, petition(2047))
	feed(cd, now, in[:3]...)
	late := now.Add(fragmentRunFor + time.Second)
	if got := handled(t, in[3:], feed(cd, late, in[3])); len(got) != 0 {
		t.Fatalf("a run past fragmentRunFor: handed on %d", len(got))
	}
	if got := feed(cd, late, wholeMsg(5, []byte{0x0B, 0x00})); got[0].v != AssemblyDeliver {
		t.Fatalf("after it: %v", got[0].v)
	}

	cd = NewConnectionData(nil, 0, 0)
	sys := make([]byte, 1400)
	in = pieces(1, sys)
	last := &protocol.Message{Type: protocol.MessageTypePing, Sequence: 2, Unsplit: true}
	last.SetData(sys[1300:])
	got := feed(cd, now, in[0], last)
	if got[1].v != AssemblyWhole || got[1].m.Type != protocol.MessageTypePing || !got[1].m.Unsplit || len(got[1].m.Data()) != 1400 {
		t.Fatalf("a system message in pieces: %v", got[1].v)
	}

	// a message whole too short for an opcode is not handed on
	cd = NewConnectionData(nil, 0, 0)
	if got := feed(cd, now, wholeMsg(1, []byte{9})); got[0].v != AssemblyDropped {
		t.Fatalf("a one-byte message: %v", got[0].v)
	}
}

// A restart that keeps the sessions (keep.go) carries a message half sent: its
// pieces so far, so the rest completes it in the new process, and the number
// the old process took last, so the client's next message is known to follow
// a message whole. The control: a session kept with no message in pieces.
func TestARestartCarriesAMessageInPieces(t *testing.T) {
	now := time.Now()
	msg := petition(2047)
	in := pieces(20, msg)
	old := NewConnectionData(nil, 0, 0)
	old.SetLastReceivedMessageSeq(21)
	feed(old, now, wholeMsg(19, []byte{0x0B, 0x00}))
	feed(old, now, in[:2]...)
	k := old.keep()
	if k.Fragments == nil || k.Fragments.Next != 22 || len(k.Fragments.Data) != 2600 {
		t.Fatalf("kept %+v", k.Fragments)
	}
	cd := NewConnectionData(nil, 0, 0)
	cd.restore(k)
	if got := handled(t, in[2:], feed(cd, now, in[2:]...)); len(got) != 1 || !bytes.Equal(got[0], msg) {
		t.Fatalf("after the restart: handed on %d", len(got))
	}

	// the control: nothing in pieces; the next message follows one whole
	old = NewConnectionData(nil, 0, 0)
	old.SetLastReceivedMessageSeq(30)
	k = old.keep()
	if k.Fragments != nil {
		t.Fatalf("kept %+v with nothing in pieces", k.Fragments)
	}
	cd = NewConnectionData(nil, 0, 0)
	cd.restore(k)
	if v, _ := cd.Assemble(pieces(31, msg)[0], false, now); v != AssemblyHeld {
		t.Fatalf("a message's first piece after the restart: %v", v)
	}
}

// The promotion to the master session starts the client's numbers over, and
// what was being put back together with them.
func TestPromotionStartsOver(t *testing.T) {
	cd := NewConnectionData(nil, 0, 0)
	now := time.Now()
	in := pieces(40, petition(2047))
	feed(cd, now, wholeMsg(39, []byte{0x0B, 0x00}))
	feed(cd, now, in[:2]...)
	cd.ResetForMasterSession()
	if v, _ := cd.Assemble(in[3], false, now); v != AssemblyUnsure {
		t.Fatalf("an old run's last piece after the reset: %v", v)
	}
	if v, _ := cd.Assemble(wholeMsg(1, []byte{0x0B, 0x00}), false, now); v != AssemblyDeliver {
		t.Fatalf("the new connection's first message: %v", v)
	}
}
