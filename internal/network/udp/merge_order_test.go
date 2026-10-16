package udp

import (
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

func rel(seq uint16) *protocol.Message {
	m := &protocol.Message{Type: protocol.MessageTypeReliable}
	m.Sequence = seq
	return m
}

func seqs(msgs []*protocol.Message) []uint16 {
	out := make([]uint16, 0, len(msgs))
	for _, m := range msgs {
		if m.Type.IsReliable() {
			out = append(out, m.Sequence)
		}
	}
	return out
}

// The regression this exists for: fragment 26 was requeued (so it counts as
// pending, not a retransmit) while 27 and 28 were due for retransmit. Sending
// the retransmits first put 27 and 28 on the wire ahead of 26 on every tick,
// and the client could never close the gap.
func TestRetransmitsDoNotJumpAheadOfPendingFragment(t *testing.T) {
	pending := []*protocol.Message{rel(26)}
	retrans := []*protocol.Message{rel(27), rel(28)}

	got := seqs(mergeReliablesInSequenceOrder(pending, retrans))
	want := []uint16{26, 27, 28}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reliable order = %v, want %v", got, want)
		}
	}
}

// The opposite case must also hold: an old lost message has to overtake newly
// queued traffic, so appending retransmits last is equally wrong.
func TestOldRetransmitOvertakesNewerPending(t *testing.T) {
	pending := []*protocol.Message{rel(30), rel(31)}
	retrans := []*protocol.Message{rel(5)}

	got := seqs(mergeReliablesInSequenceOrder(pending, retrans))
	want := []uint16{5, 30, 31}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reliable order = %v, want %v", got, want)
		}
	}
}

// Non-reliable entries (state channels, pings) must keep their slots — only
// reliable messages are ordered among themselves.
func TestNonReliablesKeepTheirPositions(t *testing.T) {
	state := &protocol.Message{Type: protocol.MessageTypeObjectUpdateChannel1}
	pending := []*protocol.Message{rel(9), state, rel(7)}

	out := mergeReliablesInSequenceOrder(pending, nil)
	if out[1] != state {
		t.Fatalf("non-reliable message moved: index 1 = %v", out[1].Type)
	}
	got := seqs(out)
	if got[0] != 7 || got[1] != 9 {
		t.Fatalf("reliable order = %v, want [7 9]", got)
	}
}
