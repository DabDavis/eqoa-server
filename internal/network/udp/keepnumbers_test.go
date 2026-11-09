package udp

import (
	"encoding/json"
	"net"
	"slices"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keptNumbersSession is a session in the world, promoted, whose message
// numbers start after first (0: at 1).
func keptNumbersSession(t *testing.T, first uint16) *session.Session {
	t.Helper()
	m := session.NewManager()
	s := m.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40123}, 0x2EC7D061, 0x2EC7D061, 0x1234, 0xB991, false)
	m.PromoteSessionToMaster(s, 500000042)
	s.SetNeedsNewInstance(false)
	s.SetState(session.StateInGame)
	s.SetInGame(true)
	s.SetCharacterInWorld(true)
	for s.Connection().CurrentMessageSeq() != first {
		s.Connection().NextMessageSeq()
	}
	return s
}

func numberedLine(b byte) *protocol.Message {
	op := protocol.OpcodeClientMessage
	m := protocol.NewMessage(protocol.MessageTypeReliable, op)
	m.SetData([]byte{byte(uint16(op)), byte(uint16(op) >> 8), b})
	return m
}

// reliableNumbers is the numbers of the reliable messages a packet carries,
// in its order.
func reliableNumbers(t *testing.T, b []byte) []uint16 {
	t.Helper()
	p, err := ParsePacket(b)
	if err != nil {
		t.Fatal(err)
	}
	var out []uint16
	for _, m := range p.Body.Messages {
		if m.Type.IsReliable() {
			out = append(out, m.Sequence)
		}
	}
	return out
}

// throughARestart is s written down, as JSON, and put back in a new manager:
// a keep restart (tools/swap_keep.sh).
func throughARestart(t *testing.T, s *session.Session) *session.Session {
	t.Helper()
	b, err := json.Marshal(s.Keep())
	if err != nil {
		t.Fatal(err)
	}
	var k session.Kept
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	r, err := session.NewManager().Resume(k)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A reliable message carried over a restart goes again under its own number
// (petition 1378: logged in again, a player could see the creatures move but
// had no chat and no cons). Since 2119f18 a restored message went out under
// the next new number and its own was never sent again, so the client, which
// delivers in order, held everything after it for good: here 2 and 3 went as
// 4 and 5, and the client, which had 1, waited for a 2 no list held. The
// control: before the restart the same queue sends its messages under their
// numbers, and after it a new message takes the next number.
func TestAMessageCarriedOverARestartGoesUnderItsOwnNumber(t *testing.T) {
	s := keptNumbersSession(t, 0)
	q := s.Queue()
	q.AddWithSequence(numberedLine(1))
	q.AddWithSequence(numberedLine(2))
	if got := reliableNumbers(t, NewRDPHandler(nil).PreparePacket(s)); !slices.Equal(got, []uint16{1, 2}) {
		t.Fatalf("the control: the first packet carries %v, want [1 2]", got)
	}
	s.Connection().SetClientLastReceivedMessage(1)
	q.RemoveReliables()
	q.AddWithSequence(numberedLine(3)) // still to go when the restart comes

	r := throughARestart(t, s)
	if got := reliableNumbers(t, NewRDPHandler(nil).PreparePacket(r)); !slices.Equal(got, []uint16{2, 3}) {
		t.Fatalf("after the restart the packet carries %v, want [2 3]: the client has 1 and waits for 2", got)
	}
	if seq := r.Queue().AddWithSequence(numberedLine(4)); seq != 4 {
		t.Fatalf("the next new message is %d, want 4", seq)
	}
}

// 0 is a number like any other through a restart too: a message numbered 0
// still to go when the restart came is ONE message in both queues, sent once,
// as 0. Restored as two (the restore took 0 for no number), it went twice in
// one packet, a segment the client refuses whole (two copies of a number,
// connection.cpp); renumbered, the client waited for a 0 that never came. The
// control: the 65535 before it, sent and acknowledged, is not sent again.
func TestAMessageNumberedZeroComesThroughARestartOnce(t *testing.T) {
	s := keptNumbersSession(t, 65534)
	q := s.Queue()
	q.AddWithSequence(numberedLine(1)) // 65535
	if got := reliableNumbers(t, NewRDPHandler(nil).PreparePacket(s)); !slices.Equal(got, []uint16{65535}) {
		t.Fatalf("the control: the first packet carries %v, want [65535]", got)
	}
	s.Connection().SetClientLastReceivedMessage(65535)
	q.RemoveReliables()
	if seq := q.AddWithSequence(numberedLine(2)); seq != 0 {
		t.Fatalf("the message after 65535 is %d, want 0", seq)
	}

	r := throughARestart(t, s)
	if got := reliableNumbers(t, NewRDPHandler(nil).PreparePacket(r)); !slices.Equal(got, []uint16{0}) {
		t.Fatalf("after the restart the packet carries %v, want [0] once", got)
	}
	if seq := r.Queue().AddWithSequence(numberedLine(3)); seq != 1 {
		t.Fatalf("the next new message is %d, want 1", seq)
	}
}
