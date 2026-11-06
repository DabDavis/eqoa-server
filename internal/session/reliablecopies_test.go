package session_test

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A copy of a reliable message the client sent again (the same number and
// contents) is not taken a second time; the same number with other contents
// is (another numbering space's, as a lobby's resend after a promotion), as
// is a copy past reliableTakenFor, and everything after the master session's
// reset (petition 1032: a shout handled 24 times off one stalled link).
func TestTakeReliableOnce(t *testing.T) {
	cd := session.NewConnectionData(nil, 1, 2)
	now := time.Date(2026, 11, 6, 3, 26, 51, 0, time.UTC)
	shout := session.ReliableSum(0xFB, 0x0BB8, []byte("agh the 2 beheaders are sick "))
	other := session.ReliableSum(0xFB, 0x002A, []byte{1, 2, 3})
	for _, st := range []struct {
		seq  uint16
		sum  uint64
		at   time.Duration
		want bool
	}{
		{0, shout, 0, true},  // 0 is never a copy
		{0, shout, 0, true},  // nor recorded
		{86, shout, 0, true}, // the shout
		{86, shout, 10 * time.Millisecond, false},
		{86, shout, 20 * time.Millisecond, false}, // every packet of the burst
		{86, other, 30 * time.Millisecond, true},  // the number again, other contents
		{87, shout, 40 * time.Millisecond, true},  // the same words said again
		{86, shout, 31 * time.Second, true},       // a resend that late is a new try
	} {
		if got := cd.TakeReliableOnce(st.seq, st.sum, now.Add(st.at)); got != st.want {
			t.Errorf("seq %d at +%v: %v, want %v", st.seq, st.at, got, st.want)
		}
	}
	if cd.LastReceivedMessageSeq() != 87 {
		t.Errorf("last received %d, want 87 (the acknowledgment's)", cd.LastReceivedMessageSeq())
	}
	cd.ResetForMasterSession()
	if !cd.TakeReliableOnce(87, shout, now.Add(32*time.Second)) || cd.LastReceivedMessageSeq() != 87 {
		t.Errorf("after the reset: a copy, or last %d", cd.LastReceivedMessageSeq())
	}
	// the window holds its last reliableTakenKept: a client's unacknowledged
	// window is far smaller, so the oldest falling out is no copy anyone sends
	for seq := uint16(100); seq < 100+300; seq++ {
		cd.TakeReliableOnce(seq, shout, now.Add(33*time.Second))
	}
	if cd.TakeReliableOnce(399, shout, now.Add(34*time.Second)) || !cd.TakeReliableOnce(100, shout, now.Add(34*time.Second)) {
		t.Errorf("the window: the newest not a copy, or the oldest still kept")
	}
}
