package udp

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// queueN queues n reliable messages of size bytes each on sess, as handlers
// queue them.
func queueN(sess *session.Session, n, size int) []*protocol.Message {
	out := make([]*protocol.Message, n)
	for i := range out {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
		data := make([]byte, size)
		data[0], data[1] = 0xE0, 0x07
		m.SetData(data)
		sess.Queue().AddWithSequence(m)
		out[i] = m
	}
	return out
}

// sent is how many of msgs went out (a send stamps LastSent past Queued).
func sent(msgs []*protocol.Message) (n int) {
	for _, m := range msgs {
		if m.LastSent.After(m.Queued) {
			n++
		}
	}
	return n
}

// The client's ack and mask retire what it holds (connection_record_
// guaranteed_ack's rule: at or before the ack, or a bit of the mask, bit i
// the message ack+2+i), so the resends are the messages it lacks, not every
// one past the hole (the server read the mask as zigzag and never used it).
func TestTheClientsMaskRetiresWhatItHolds(t *testing.T) {
	sess := session.NewSession(nil, 9, 9, 1, 2, true)
	queueN(sess, 8, 10)
	q := sess.Queue()
	q.GetPending()
	// it holds 1-2, then 4 and 6 past the hole at 3 (bits 0 and 2)
	r := NewRDPHandler(nil)
	r.ProcessIncoming(sess, &Packet{Header: &protocol.SegmentHeader{Flags: protocol.SegmentFlagIsRemote}, Body: &protocol.SegmentBody{
		Flags:         protocol.BodyFlagGuaranteedAck | protocol.BodyFlagGuaranteedAckMask,
		ArrivalNumber: 1, GuaranteedAck: 2, GuaranteedAckMask: 0b101,
	}})
	if got := seqs(q.GetUnacknowledged()); len(got) != 4 || got[0] != 3 || got[1] != 5 || got[2] != 7 || got[3] != 8 {
		t.Fatalf("left after ack 2 with 4 and 6 held: %v, want [3 5 7 8]", got)
	}
	// an older packet's word (ack 1) takes nothing back
	r.ProcessIncoming(sess, &Packet{Header: &protocol.SegmentHeader{Flags: protocol.SegmentFlagIsRemote}, Body: &protocol.SegmentBody{
		Flags: protocol.BodyFlagGuaranteedAck, ArrivalNumber: 2, GuaranteedAck: 1,
	}})
	if got := seqs(q.GetUnacknowledged()); len(got) != 4 {
		t.Fatalf("an older ack brought messages back: %v", got)
	}
}

// A resend waits the round trip's timeout, twice as long each time it went
// again (at most four times, never past maxResendWait), and a packet's
// resends fill at most ResendBudget bytes, the first whatever its size: a
// client gone quiet costs a packet every few seconds, not four messages every
// tick (eqoaclassic's dead sessions: 600 lines a minute each).
func TestResendsBackOffAndKeepToTheirBudget(t *testing.T) {
	sess := session.NewSession(nil, 9, 9, 1, 2, true)
	q := sess.Queue()
	frags := queueN(sess, 3, 1156)
	q.GetPending()
	rto := 300 * time.Millisecond
	past := time.Now().Add(-rto - 10*time.Millisecond)
	for _, m := range frags {
		m.LastSent = past
	}
	got := q.TakeResends(rto, MaxRetransmitsPerPacket, ResendBudget)
	if len(got) != 1 || got[0] != frags[0] {
		t.Fatalf("three fragments due, budget %d: took %d, want the oldest alone", ResendBudget, len(got))
	}
	// resent once: not due again at one timeout, due at two
	frags[0].LastSent = time.Now().Add(-rto - 10*time.Millisecond)
	if again := q.TakeResends(rto, MaxRetransmitsPerPacket, ResendBudget); len(again) != 1 || again[0] != frags[1] {
		t.Fatal("the fragment resent once went again after one timeout")
	}
	frags[0].LastSent = time.Now().Add(-2*rto - 10*time.Millisecond)
	if again := q.TakeResends(rto, MaxRetransmitsPerPacket, ResendBudget); len(again) != 1 || again[0] != frags[0] {
		t.Fatal("the fragment resent once did not go again after two timeouts")
	}

	// small messages share a packet's budget
	sess = session.NewSession(nil, 9, 9, 1, 2, true)
	q = sess.Queue()
	small := queueN(sess, 20, 12)
	q.GetPending()
	for _, m := range small {
		m.LastSent = past
	}
	if got := q.TakeResends(rto, MaxRetransmitsPerPacket, ResendBudget); len(got) != MaxRetransmitsPerPacket {
		t.Fatalf("20 small messages due: took %d, want %d", len(got), MaxRetransmitsPerPacket)
	}
}

// A reliable message goes out only within SendWindow of the client's ack;
// the rest wait in the queue, and a tick sends more packets while messages
// wait within it (BurstPackets): a MemoryDump went one fragment a tick, ten a
// second, and everything queued behind it waited that long. The control: the
// window moves with the ack.
func TestTheWindowAndTheBurst(t *testing.T) {
	sess := session.NewSession(nil, 9, 9, 1, 2, true)
	msgs := queueN(sess, 120, 300)
	r := NewRDPHandler(nil)
	// as TickSession: the packet, then the burst's
	for i := 0; i < 1+BurstPackets*4; i++ {
		ack, _ := sess.Connection().ClientAck()
		if i > 0 && !sess.Queue().Sendable(ack, SendWindow) {
			break
		}
		if r.PreparePacket(sess) == nil {
			t.Fatal("no packet")
		}
	}
	if n := sent(msgs); n != SendWindow {
		t.Fatalf("%d sent with no ack, want the window's %d", n, SendWindow)
	}
	if sent(msgs[SendWindow:]) != 0 {
		t.Fatal("a message past the window went out")
	}
	// the client acknowledges 40: 40 more may go
	sess.Connection().SetClientAck(40, 0)
	sess.Queue().RemoveReliables()
	for i := 0; i < 20; i++ {
		ack, _ := sess.Connection().ClientAck()
		if !sess.Queue().Sendable(ack, SendWindow) {
			break
		}
		r.PreparePacket(sess)
	}
	if n := sent(msgs); n != 40+SendWindow {
		t.Fatalf("after the ack of 40: %d sent, want %d", n, 40+SendWindow)
	}
}

// Our packets acknowledge the client's messages as CLIENT reads an ack: the
// newest taken with every one before it, and those past it in the ack's mask
// (body flag 0x08, a plain varint after the guaranteed ack).
func TestOurAckSaysTheHoleAndTheMask(t *testing.T) {
	// a lobby session: its header is the one ParsePacket reads (a promoted
	// one's carries the SessionID as the client reads its src)
	sess := session.NewSession(nil, 9, 9, 1, 2, false)
	conn := sess.Connection()
	for _, seq := range []uint16{1, 2, 4, 5} {
		conn.TakeReliable(seq)
	}
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
	data := NewRDPHandler(nil).PreparePacket(sess)
	pkt, err := ParsePacket(data)
	if err != nil {
		t.Fatalf("our packet does not parse: %v", err)
	}
	b := pkt.Body
	if b.Flags&protocol.BodyFlagGuaranteedAckMask == 0 || b.GuaranteedAck != 2 || b.GuaranteedAckMask != 0b11 {
		t.Fatalf("flags 0x%X ack %d mask %b, want 0x08 set, ack 2 (the hole at 3), mask 11 (4 and 5)", b.Flags, b.GuaranteedAck, b.GuaranteedAckMask)
	}
	// the hole filled: no mask
	conn.TakeReliable(3)
	sess.Queue().SetBodyFlags(protocol.BodyFlagGuaranteedAck)
	pkt, err = ParsePacket(NewRDPHandler(nil).PreparePacket(sess))
	if err != nil {
		t.Fatal(err)
	}
	if b := pkt.Body; b.Flags&protocol.BodyFlagGuaranteedAckMask != 0 || b.GuaranteedAck != 5 {
		t.Fatalf("after 3: flags 0x%X ack %d, want no mask and 5", b.Flags, b.GuaranteedAck)
	}
}

// A client in the world silent StateSilence gets no state channels until it
// is heard from again (its dropped session lingers its 50 pings and was sent
// the world every tick, to a relay port long closed); a session silent
// PreGameSilentEnd before the world ends (it lived 10 pings of 300 s). The
// controls: heard from, it gets them again; a silent session in the world is
// not ended by this.
func TestASilentClientIsSentNoWorldAndASilentLobbyEnds(t *testing.T) {
	r, sess, cli := slotsTick(t, 24)
	if got := tick(t, r, sess, cli); len(got) == 0 || len(got[0]) == 0 {
		t.Fatal("the control: a client heard from gets its entity channels")
	}
	sess.SetLastActivity(time.Now().Add(-StateSilence - time.Second))
	for _, chs := range tick(t, r, sess, cli) {
		if len(chs) != 0 {
			t.Fatalf("a client silent past %v was sent entity channels %v", StateSilence, chs)
		}
	}
	if sess.PendingTermination() {
		t.Fatal("a silent session in the world ended (its pings decide that)")
	}
	sess.UpdateActivity()
	for slot := 1; slot < 24; slot++ { // the world moves on
		v := make([]byte, 162)
		v[0], v[5] = 1, 0x82
		binary.LittleEndian.PutUint32(v[1:], uint32(100000+slot))
		v[20] = byte(slot)
		sess.Connection().GetServerObject(slot).SetData(v)
	}
	if got := tick(t, r, sess, cli); len(got) == 0 || len(got[0]) == 0 {
		t.Fatal("heard from again, it is sent nothing")
	}

	lobby := session.NewSession(cli.LocalAddr().(*net.UDPAddr), 2, 2, 1, 2, false)
	lobby.SetLastActivity(time.Now().Add(-PreGameSilentEnd + time.Minute))
	r.TickSession(lobby)
	if lobby.PendingTermination() {
		t.Fatal("a lobby session two minutes silent ended")
	}
	lobby.SetLastActivity(time.Now().Add(-PreGameSilentEnd - time.Second))
	r.TickSession(lobby)
	if !lobby.PendingTermination() {
		t.Fatalf("a lobby session silent past %v lives on", PreGameSilentEnd)
	}
}
