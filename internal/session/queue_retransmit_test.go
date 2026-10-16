package session

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// reliableMsg builds a reliable message with a sequence and last-sent time,
// bypassing AddWithSequence so the test needs no live Session/Connection.
func reliableMsg(seq uint16, lastSent time.Time) *protocol.Message {
	m := &protocol.Message{Type: protocol.MessageTypeReliable}
	m.Sequence = seq
	m.LastSent = lastSent
	return m
}

// A message still awaiting its FIRST send must never be treated as a
// retransmit — otherwise it goes out twice in the same packet.
func TestTakeRetransmitsSkipsUnsentPending(t *testing.T) {
	old := time.Now().Add(-time.Second)
	q := &MessageQueue{}

	msg := reliableMsg(7, old)
	q.pending = []*protocol.Message{msg}
	q.reliable = []*protocol.Message{msg}

	if q.HasRetransmits(200 * time.Millisecond) {
		t.Fatal("a message still queued for its first send was reported as due for retransmit")
	}
	if got := q.TakeRetransmits(200*time.Millisecond, 4); len(got) != 0 {
		t.Fatalf("expected no retransmits, got %d", len(got))
	}
}

// The gap case this whole change exists for: a sent-but-unacked message must
// come back once the delay elapses.
func TestTakeRetransmitsReturnsStaleUnacked(t *testing.T) {
	delay := 200 * time.Millisecond
	stale := time.Now().Add(-time.Second)
	fresh := time.Now()

	q := &MessageQueue{}
	q.reliable = []*protocol.Message{
		reliableMsg(8, stale), // lost — must be resent
		reliableMsg(9, fresh), // just sent — must not be
	}

	if !q.HasRetransmits(delay) {
		t.Fatal("stale unacked message was not reported as due")
	}
	got := q.TakeRetransmits(delay, 4)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 retransmit, got %d", len(got))
	}
	if got[0].Sequence != 8 {
		t.Fatalf("expected seq 8 (the stale one), got %d", got[0].Sequence)
	}

	// Taking must re-stamp it, so it is not resent again on the next tick.
	if q.HasRetransmits(delay) {
		t.Fatal("message was still due immediately after being taken — LastSent not updated")
	}
}

// Recovery must not itself become a burst.
func TestTakeRetransmitsRespectsCap(t *testing.T) {
	stale := time.Now().Add(-time.Second)
	q := &MessageQueue{}
	for seq := uint16(1); seq <= 10; seq++ {
		q.reliable = append(q.reliable, reliableMsg(seq, stale))
	}

	got := q.TakeRetransmits(200*time.Millisecond, 4)
	if len(got) != 4 {
		t.Fatalf("expected the cap of 4, got %d", len(got))
	}
	// Lowest sequences first: the client cannot advance past the earliest gap.
	for i, m := range got {
		if want := uint16(i + 1); m.Sequence != want {
			t.Fatalf("retransmit %d: expected seq %d, got %d", i, want, m.Sequence)
		}
	}
}
