package session_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/eqoa/server/internal/session"
)

// drdpReceiver is connection.cpp's receive half for one state channel, all of
// it (connection_parse_segment_body's state branch and
// connection_record_state_message), where sizedchannel_test.go's receiver
// keeps only the XOR: a refnum above REFNUM_OFFSET_MAX, or one naming a
// message no longer held, refuses the whole segment; a sequence at or below rx
// is a duplicate, dropped but acknowledged again; an accepted message becomes
// rx and drops what is older than its reference (32 back for a whole value).
type drdpReceiver struct {
	rx   uint16
	held []heldValue
}

type heldValue struct {
	seq  uint16
	data []byte
}

// receive takes one message. ok is whether its segment survived, and value
// the value it carried when it was new (nil for a duplicate).
func (r *drdpReceiver) receive(data []byte, seq uint16, ref byte) (value []byte, ok bool) {
	if ref > session.RefnumOffsetMax {
		return nil, false
	}
	if int16(seq-r.rx) <= 0 {
		return nil, true
	}
	value = append([]byte(nil), data...)
	if ref != 0 {
		var base []byte
		found := false
		for _, h := range r.held {
			if h.seq == seq-uint16(ref) {
				base, found = h.data, true
			}
		}
		if !found {
			return nil, false
		}
		for i := 0; i < len(value) && i < len(base); i++ {
			value[i] ^= base[i]
		}
	}
	r.rx = seq
	keep := seq - uint16(ref)
	if ref == 0 {
		keep = seq - 32
	}
	for len(r.held) > 0 && int16(r.held[0].seq-keep) < 0 {
		r.held = r.held[1:]
	}
	r.held = append(r.held, heldValue{seq, value})
	return value, true
}

// entityValue is an object channel's value as the player's own channel 0
// carries it: flag 1, the id, type 0x82, and position bytes that move.
func entityValue(step int) []byte {
	v := make([]byte, 162)
	v[0] = 1
	v[1], v[2], v[3], v[4] = 0x05, 0x65, 0xCD, 0x1D
	v[5] = 0x82
	v[6], v[7], v[8] = byte(step>>16), byte(step>>8), byte(step)
	v[27] = byte(step * 7)
	return v
}

// sender drives one channel and remembers the value each sequence number
// carried, which is what the client must hold for it.
type sender struct {
	t    *testing.T
	st   *session.StateUpdateData
	sent map[uint16][]byte
}

func newSender(t *testing.T) *sender {
	return &sender{t: t, st: session.NewConnectionData(nil, 0, 0).GetServerObject(0), sent: map[uint16][]byte{}}
}

// change sets a new value and returns the message it goes out as.
func (s *sender) change(v []byte) (data []byte, seq uint16, ref byte) {
	s.t.Helper()
	s.st.SetData(v)
	data, seq, ref = s.st.GenerateXORDelta()
	if data == nil {
		s.t.Fatal("a changed value sent nothing")
	}
	if ref > session.RefnumOffsetMax {
		s.t.Fatalf("seq %d went out with refnum %d: CLIENT refuses the whole segment above %d", seq, ref, session.RefnumOffsetMax)
	}
	s.sent[seq] = append([]byte(nil), v...)
	return data, seq, ref
}

// deliver hands a message to the client and checks what it then holds.
func (s *sender) deliver(c *drdpReceiver, data []byte, seq uint16, ref byte) bool {
	s.t.Helper()
	got, ok := c.receive(data, seq, ref)
	if got != nil && !bytes.Equal(got, s.sent[seq]) {
		s.t.Fatalf("seq %d (refnum %d): the client decoded a value the server never sent (flag %d, type %#x)", seq, ref, got[0], got[5])
	}
	return ok
}

type message struct {
	data []byte
	seq  uint16
	ref  byte
}

// TestStateChannelSurvivesAStall is F3 20261024-001134. The page stopped for
// two minutes (a hidden tab) while the player's channel 0 kept changing, then
// took the backlog in one go and acked it in a burst. Every message must keep
// a refnum the client accepts and decode to what was sent; the old base model
// let the refnum pass 32 (every segment refused) and wrap to 0, a delta taken
// for a whole value whose flag, id and type XOR to zero: the player's own
// entity removed, for good.
func TestStateChannelSurvivesAStall(t *testing.T) {
	s := newSender(t)
	c := &drdpReceiver{}
	step := 0
	// in step: each message acked as it lands
	for ; step < 20; step++ {
		data, seq, ref := s.change(entityValue(step))
		if !s.deliver(c, data, seq, ref) {
			t.Fatalf("seq %d refused in step", seq)
		}
		s.st.UpdateBaseXor(c.rx)
	}
	// the stall: the server goes on changing the value, nothing arrives
	var backlog []message
	for ; step < 20+400; step++ {
		data, seq, ref := s.change(entityValue(step))
		backlog = append(backlog, message{data, seq, ref})
	}
	// the backlog in one go; acks come back in bursts, behind the sends
	var acks []uint16
	for i, m := range backlog {
		if s.deliver(c, m.data, m.seq, m.ref) {
			acks = append(acks, c.rx)
		}
		if i%7 == 6 {
			for _, a := range acks {
				s.st.UpdateBaseXor(a)
			}
			acks = acks[:0]
			data, seq, ref := s.change(entityValue(step))
			step++
			if !s.deliver(c, data, seq, ref) {
				t.Fatalf("seq %d (refnum %d) refused after the stall", seq, ref)
			}
		}
	}
	for _, a := range acks {
		s.st.UpdateBaseXor(a)
	}
	// and after: in step again, every message accepted
	for i := 0; i < 50; i++ {
		data, seq, ref := s.change(entityValue(step))
		step++
		if !s.deliver(c, data, seq, ref) {
			t.Fatalf("seq %d (refnum %d) refused after the stall: the channel never recovered", seq, ref)
		}
		s.st.UpdateBaseXor(c.rx)
	}
	if last := c.held[len(c.held)-1].data; !bytes.Equal(last, entityValue(step-1)) || last[0] != 1 || last[5] != 0x82 {
		t.Fatal("the client does not hold the player's current value")
	}
}

// Acks for messages that were in flight together each move the reference: the
// newest acknowledged is the one used. The old model threw away every pending
// delta at the first ack, so the later ones were ignored and the reference
// stayed behind while the refnum grew.
func TestLaterAckOfAnInFlightMessageMovesTheReference(t *testing.T) {
	s := newSender(t)
	c := &drdpReceiver{}
	var flight []message
	for step := 0; step < 30; step++ {
		data, seq, ref := s.change(entityValue(step))
		flight = append(flight, message{data, seq, ref})
		s.deliver(c, data, seq, ref)
	}
	s.st.UpdateBaseXor(flight[4].seq)
	s.st.UpdateBaseXor(flight[29].seq)
	data, seq, ref := s.change(entityValue(30))
	if ref != 1 {
		t.Fatalf("after acks of seq %d and %d, seq %d went out with refnum %d, want 1 (against the newer)", flight[4].seq, flight[29].seq, seq, ref)
	}
	if !s.deliver(c, data, seq, ref) {
		t.Fatal("refused")
	}
	// an older ack after a newer one moves nothing back
	s.st.UpdateBaseXor(flight[10].seq)
	if _, seq, ref := s.change(entityValue(31)); ref != byte(seq-flight[29].seq) {
		t.Fatalf("an old ack moved the reference: refnum %d", ref)
	}
}

// Past RefnumOffsetMax unacknowledged messages the value goes out whole, and
// the client takes it.
func TestUnacknowledgedChannelGoesOutWhole(t *testing.T) {
	s := newSender(t)
	c := &drdpReceiver{}
	data, seq, ref := s.change(entityValue(0))
	s.deliver(c, data, seq, ref)
	s.st.UpdateBaseXor(seq)
	for step := 1; step <= 100; step++ {
		data, seq, ref := s.change(entityValue(step))
		if step <= session.RefnumOffsetMax && ref != byte(step) {
			t.Fatalf("step %d: refnum %d, want %d (against the one acknowledged)", step, ref, step)
		}
		if step > session.RefnumOffsetMax && ref != 0 {
			t.Fatalf("step %d: refnum %d, want 0 (the acknowledged message is too far back)", step, ref)
		}
		if !s.deliver(c, data, seq, ref) {
			t.Fatalf("step %d refused", step)
		}
	}
}

// Random loss of messages and acks, delayed and reordered acks, resends, and a
// sequence number that passes 65535: nothing the client accepts may differ
// from what was sent, and once the loss stops the client catches up.
func TestStateChannelUnderLossAndWrap(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	s := newSender(t)
	c := &drdpReceiver{}
	var acks []uint16
	value := entityValue(0)
	for step := 0; step < 80000; step++ {
		lossy := step < 78000
		changed := rng.Intn(10) != 0
		if changed {
			value = entityValue(step)
			s.st.SetData(value)
		}
		data, seq, ref := s.st.GenerateXORDelta()
		if data != nil {
			if ref > session.RefnumOffsetMax {
				t.Fatalf("step %d: refnum %d", step, ref)
			}
			if changed {
				// a new message (its number may be one used before the wrap)
				s.sent[seq] = append([]byte(nil), value...)
			}
			if !lossy || rng.Intn(10) != 0 {
				if s.deliver(c, data, seq, ref) && (!lossy || rng.Intn(5) != 0) {
					acks = append(acks, c.rx)
				}
			}
		}
		// acks land late, in bursts, sometimes out of order
		if len(acks) > 0 && (!lossy || rng.Intn(4) == 0) {
			if lossy && len(acks) > 1 && rng.Intn(3) == 0 {
				acks[0], acks[1] = acks[1], acks[0]
			}
			for _, a := range acks {
				s.st.UpdateBaseXor(a)
			}
			acks = acks[:0]
		}
	}
	if len(c.held) == 0 || !bytes.Equal(c.held[len(c.held)-1].data, value) {
		t.Fatal("after the loss stopped the client did not catch up with the server's value")
	}
	if s.st.GetSeqNum() > 10000 {
		t.Fatalf("the sequence number never wrapped (%d): the test missed the wrap", s.st.GetSeqNum())
	}
}

// The client's own channel (0x40) keeps every base it can still name across
// the sequence number's wrap: the old eviction dropped the numerically
// smallest, which past 65535 were the newest, so nothing decoded again.
func TestClientBasesAcrossTheWrap(t *testing.T) {
	c := session.NewConnectionData(nil, 0, 0).GetClientState()
	for seq := uint16(65500); seq != 100; seq++ {
		if !c.Newer(seq) {
			t.Fatalf("seq %d counted as a duplicate", seq)
		}
		c.AddBase(seq, []byte{byte(seq), byte(seq >> 8)})
		c.Took(seq)
		for back := uint16(1); back <= session.RefnumOffsetMax; back++ {
			if seq-back == 65500-1 {
				break
			}
			if got := c.GetBase(seq - back); got == nil || got[0] != byte(seq-back) {
				t.Fatalf("at seq %d the base %d back (%d) is gone", seq, back, seq-back)
			}
		}
	}
	if c.Newer(99) || c.Newer(65535) {
		t.Fatal("a message older than the last taken counts as new")
	}
}
