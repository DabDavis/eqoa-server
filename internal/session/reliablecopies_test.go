package session_test

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// The client's reliable messages are taken once each by number, in a window
// past the ack (rxwindow.go): a copy (every packet of a stalled link's burst,
// petition 1032's shout handled 24 times) is not taken again, however late;
// one past a hole is taken and goes in the ack's mask, the ack staying at the
// hole (the server acknowledged the newest taken until 2026-11-09, and CLIENT
// retired the one lost on its way unseen); the hole filled takes the ack past
// all of them; one beyond the 64 the mask can name waits for the client's
// next try. After the master session's reset a new connection's first is
// new. The control of the mask: in order, none.
func TestTakeReliableOnceByTheWindow(t *testing.T) {
	cd := session.NewConnectionData(nil, 1, 2)
	now := time.Date(2026, 11, 6, 3, 26, 51, 0, time.UTC)
	shout := session.ReliableSum(0xFB, 0x0BB8, []byte("agh the 2 beheaders are sick "))
	for _, st := range []struct {
		seq       uint16
		at        time.Duration
		want      bool
		ack       uint16
		mask      uint64
		reasonFor string
	}{
		{1, 0, true, 1, 0, "the first"},
		{1, 10 * time.Millisecond, false, 1, 0, "a copy"},
		{2, 20 * time.Millisecond, true, 2, 0, "in order: no mask"},
		{4, 30 * time.Millisecond, true, 2, 0b1, "past a hole (3): the ack at the hole, 4 in the mask"},
		{6, 40 * time.Millisecond, true, 2, 0b101, "6 too"},
		{4, 50 * time.Millisecond, false, 2, 0b101, "a copy of one in the mask"},
		{3, 60 * time.Millisecond, true, 4, 0b1, "the hole filled: the ack past 4, 6 still in the mask"},
		{5, 70 * time.Millisecond, true, 6, 0, "and the next: the ack past 6"},
		{2, 31 * time.Second, false, 6, 0, "a copy that late is still a copy"},
		{6 + 66, 32 * time.Second, false, 6, 0, "beyond the window: not now"},
		{7, 33 * time.Second, true, 7, 0, "the next"},
	} {
		if got := cd.TakeReliableOnce(st.seq, shout, now.Add(st.at)); got != st.want {
			t.Fatalf("seq %d (%s): %v, want %v", st.seq, st.reasonFor, got, st.want)
		}
		r := cd.NextPacketReport()
		if r.Ack != st.ack || r.Mask != st.mask {
			t.Fatalf("seq %d (%s): ack %d mask %b, want %d %b", st.seq, st.reasonFor, r.Ack, r.Mask, st.ack, st.mask)
		}
	}
	cd.ResetForMasterSession()
	if !cd.TakeReliableOnce(1, shout, now.Add(40*time.Second)) || cd.LastReceivedMessageSeq() != 1 {
		t.Fatalf("after the reset: the new connection's first a copy, or last %d", cd.LastReceivedMessageSeq())
	}
	// past 65535
	cd = session.NewConnectionData(nil, 1, 2)
	cd.SetLastReceivedMessageSeq(65534)
	for _, seq := range []uint16{65535, 1, 0} {
		if !cd.TakeReliableOnce(seq, shout, now) {
			t.Fatalf("seq %d past 65534 not new", seq)
		}
	}
	if r := cd.NextPacketReport(); r.Ack != 1 || r.Mask != 0 {
		t.Fatalf("past 65535: ack %d mask %b, want 1 and none", r.Ack, r.Mask)
	}
	if cd.TakeReliableOnce(65535, shout, now) {
		t.Fatal("a copy of 65535 taken after the wrap")
	}
}
