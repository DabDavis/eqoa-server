package udp

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// streamRig is a listener on a socket of its own and a client's session at
// character select (not in the game, no character in the world), with the
// socket its datagrams reach.
type streamRig struct {
	t    *testing.T
	l    *Listener
	sm   *session.Manager
	sess *session.Session
	cli  *net.UDPConn
}

const streamInstance = 0x3EA84596

// streamFill is what a long message's body is made of (long): a packet that
// carries a piece of one holds a run of it.
const streamFill = 0xA5

func newStreamRig(t *testing.T) *streamRig {
	t.Helper()
	lo := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}
	srv, err := net.ListenUDP("udp4", lo)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := net.ListenUDP("udp4", lo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close(); cli.Close() })
	sm := session.NewManager()
	l := NewListener(nil, sm, nil, nil)
	l.conn = srv
	sess := sm.CreateSession(cli.LocalAddr().(*net.UDPAddr), streamInstance, streamInstance, 0x4595, 0xB991, false)
	return &streamRig{t: t, l: l, sm: sm, sess: sess, cli: cli}
}

// long queues a reliable message of fragments pieces (the worlds' recipes are
// 62) and returns its last number.
func (r *streamRig) long(fragments int) uint16 {
	body := bytes.Repeat([]byte{streamFill}, fragments*session.MaxFragmentSize-10)
	op := uint16(protocol.OpcodeServerWorldsData)
	body[0], body[1] = byte(op), byte(op>>8)
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerWorldsData)
	m.SetData(body)
	return r.sess.Queue().AddWithSequence(m)
}

// read is the reliable messages' numbers in each datagram the client's socket
// was sent since the last read.
func (r *streamRig) read() [][]uint16 {
	r.t.Helper()
	var out [][]uint16
	buf := make([]byte, 65536)
	for {
		r.cli.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
		n, _, err := r.cli.ReadFromUDP(buf)
		if err != nil {
			return out
		}
		p, err := ParsePacket(append([]byte(nil), buf[:n]...))
		if err != nil {
			r.t.Fatalf("datagram %d does not parse: %v", len(out), err)
		}
		seqs := []uint16{}
		if p.Body != nil {
			for _, m := range p.Body.Messages {
				if m.Type.IsReliable() {
					seqs = append(seqs, m.Sequence)
				}
			}
		}
		out = append(out, seqs)
	}
}

// flat is every number in got, in order.
func flat(got [][]uint16) []uint16 {
	var out []uint16
	for _, d := range got {
		out = append(out, d...)
	}
	return out
}

func inOrderFrom(got []uint16, first uint16) bool {
	for i, s := range got {
		if s != first+uint16(i) {
			return false
		}
	}
	return true
}

// A client at character select that took the server's worlds is sent what
// waits in its queue on the tick: 1+BurstPackets datagrams a tick, a fragment
// each, in order, until the client's window is full (SendWindowBytes out and
// not acknowledged), then nothing, not an empty datagram, until its ack moves;
// the ack's own answer carries one fragment, as ever, and the next tick the
// burst again. The control: until it takes the offer the same session is
// sent nothing on a tick however much waits (a PS2's session, and every
// session before 2026-11-10: a packet only in answer to one of the client's,
// which made the recipes' 62 fragments a fragment for each acknowledgement).
func TestTheWorldsRecipesStreamOnTheTick(t *testing.T) {
	r := newStreamRig(t)
	last := r.long(40)
	if last != 40 {
		t.Fatalf("the message's last number is %d, want 40 fragments", last)
	}
	for k := 0; k < 3; k++ {
		r.l.tickAllSessions()
	}
	if got := r.read(); len(got) != 0 {
		t.Fatalf("the control: a session that took no offer was sent %d datagrams on its ticks: %v", len(got), got)
	}

	r.sess.TakeServerWorlds()
	r.l.tickAllSessions()
	got := r.read()
	if len(got) != 1+BurstPackets || len(flat(got)) != 1+BurstPackets || !inOrderFrom(flat(got), 1) {
		t.Fatalf("a tick: %v, want %d datagrams, fragments 1 to %d one each", got, 1+BurstPackets, 1+BurstPackets)
	}
	// on until the window is full: no more than SendWindowBytes out
	sent := len(flat(got))
	for k := 0; k < 10; k++ {
		r.l.tickAllSessions()
		more := r.read()
		for _, d := range more {
			if len(d) == 0 {
				t.Fatalf("tick %d: an empty datagram among %v", k, more)
			}
		}
		if !inOrderFrom(flat(more), uint16(sent+1)) {
			t.Fatalf("tick %d: %v, want the fragments after %d in order", k, more, sent)
		}
		sent += len(flat(more))
	}
	each := session.MaxFragmentSize + 8
	if want := SendWindowBytes / each; sent != want || r.sess.Queue().InFlightBytes() > SendWindowBytes {
		t.Fatalf("%d fragments out (%d bytes), want the window's %d", sent, r.sess.Queue().InFlightBytes(), want)
	}
	r.l.tickAllSessions()
	if got := r.read(); len(got) != 0 {
		t.Fatalf("the window full: a tick sent %v", got)
	}

	// the client acknowledges them all: its packet's own answer, then the tick's burst
	r.l.handlePacket(clientSegment(0x7000, streamInstance, 0, 1, uint16(sent), uint16(sent)), r.cli.LocalAddr().(*net.UDPAddr), nil)
	if got := r.read(); len(got) != 1 || len(got[0]) != 1 || got[0][0] != uint16(sent+1) {
		t.Fatalf("the ack's answer: %v, want one datagram with fragment %d", got, sent+1)
	}
	r.l.tickAllSessions()
	if got := r.read(); len(got) != 1+BurstPackets || !inOrderFrom(flat(got), uint16(sent+2)) {
		t.Fatalf("the tick after the ack: %v, want %d datagrams from fragment %d", got, 1+BurstPackets, sent+2)
	}
}

// A resend that is due goes out on such a session's tick while its client
// was heard from within StateSilence (a recipe's last fragment lost: the
// client, sent nothing more, says nothing more until its keepalive), one
// datagram of it; a client not heard from that long is gone and is sent
// nothing from the tick, however much is unacknowledged. The control: a
// session that took no offer is sent no resend on a tick either way.
func TestAStreamsResendWaitsOnALiveClient(t *testing.T) {
	r := newStreamRig(t)
	r.long(3)
	r.sess.TakeServerWorlds()
	r.sess.UpdateActivity()
	r.l.tickAllSessions()
	if got := flat(r.read()); len(got) != 3 || !inOrderFrom(got, 1) {
		t.Fatalf("the stream: %v, want fragments 1 to 3", got)
	}
	due := func() {
		for _, m := range r.sess.Queue().GetUnacknowledged() {
			m.LastSent = time.Now().Add(-10 * time.Second)
		}
	}
	due()
	r.l.tickAllSessions()
	if got := r.read(); len(got) != 1 || len(got[0]) == 0 || got[0][0] != 1 {
		t.Fatalf("a resend due, the client heard from: %v, want one datagram from fragment 1", got)
	}
	due()
	r.sess.SetLastActivity(time.Now().Add(-StateSilence - time.Second))
	r.l.tickAllSessions()
	if got := r.read(); len(got) != 0 {
		t.Fatalf("a resend due, the client gone: %v, want nothing", got)
	}

	ps2 := newStreamRig(t)
	ps2.long(1)
	ps2.sess.UpdateActivity()
	ps2.l.flushSession(ps2.sess)
	if got := flat(ps2.read()); len(got) != 1 {
		t.Fatalf("the control's message: %v", got)
	}
	for _, m := range ps2.sess.Queue().GetUnacknowledged() {
		m.LastSent = time.Now().Add(-10 * time.Second)
	}
	ps2.l.tickAllSessions()
	if got := ps2.read(); len(got) != 0 {
		t.Fatalf("the control: a session that took no offer was sent a resend on its tick: %v", got)
	}
}

// A packet is never built across a promotion: a pick that comes while the
// tick (or another packet's answer) is building the session's packet waits
// for it, so the packet is the lobby connection's whole, its header and the
// fragment it drained, and the promoted connection's first packet carries its
// own first message alone. Without the lock the promotion ran between the
// queue's drain and the header: the lobby's fragment went out under the
// promoted connection's header and the number of a message still to come.
func TestAPacketIsNotBuiltAcrossAPromotion(t *testing.T) {
	r := newStreamRig(t)
	r.long(3)
	rdp := NewRDPHandler(r.l)
	done := make(chan struct{})
	inside := false
	testAfterDrain = func(s *session.Session) {
		testAfterDrain = nil
		go func() {
			r.sm.PromoteSessionToMaster(r.sess, 0x1DCD650C)
			close(done)
		}()
		select {
		case <-done:
			inside = true
		case <-time.After(60 * time.Millisecond):
		}
	}
	t.Cleanup(func() { testAfterDrain = nil })
	data := rdp.PreparePacket(r.sess)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the promotion never came once the packet was built")
	}
	if inside {
		t.Fatal("the session was promoted while its packet was being built")
	}
	p, err := ParsePacket(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Header.Flags&protocol.SegmentFlagDidServerInitiate != 0 || p.Header.Flags&protocol.SegmentFlagNewInstance != 0 {
		t.Fatalf("the packet built before the promotion has the promoted connection's header: %#x", uint32(p.Header.Flags))
	}
	if len(p.Body.Messages) != 1 || p.Body.Messages[0].Sequence != 1 || p.Body.Messages[0].Type != protocol.MessageTypeSegmentReliable {
		t.Fatalf("the lobby's packet: %d messages, want the recipes' first fragment", len(p.Body.Messages))
	}
	if !r.sess.DidServerInitiate() || r.sess.Queue().PendingCount() != 0 || r.sess.Queue().ReliableCount() != 0 {
		t.Fatalf("after the promotion: promoted %v, %d pending, %d unacknowledged", r.sess.DidServerInitiate(), r.sess.Queue().PendingCount(), r.sess.Queue().ReliableCount())
	}
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
	m.SetData([]byte{0xE0, 0x07})
	r.sess.Queue().AddWithSequence(m)
	// (ParsePacket reads a client's packets: of a promoted server packet, whose
	// header carries the SessionID too, it reads the header's flags alone)
	own := rdp.PreparePacket(r.sess)
	first, err := ParsePacket(own)
	if err != nil {
		t.Fatal(err)
	}
	if first.Header.Flags&protocol.SegmentFlagDidServerInitiate == 0 || first.Header.Flags&protocol.SegmentFlagNewInstance == 0 {
		t.Fatalf("the promoted connection's first packet: header %#x", uint32(first.Header.Flags))
	}
	if bytes.Contains(own, bytes.Repeat([]byte{streamFill}, 16)) || !bytes.Contains(own, []byte{0xFB, 0x02, 0x01, 0x00, 0xE0, 0x07}) || len(own) > 64 {
		t.Fatalf("the promoted connection's first packet (%d bytes) is not its own first message (number 1) alone: %x", len(own), own[:min(len(own), 64)])
	}
	if !bytes.Contains(data, bytes.Repeat([]byte{streamFill}, 16)) {
		t.Fatal("the control: the lobby's packet holds no run of the recipes' bytes")
	}
}

// A long message waits in the queue (MessageQueue.Waits) from its queueing
// until the client holds its last fragment, sent or not: what keeps the
// worlds' recipes to one copy in a queue however often a client asks for them
// (handlers.ServerWorldsHandler.HandleOffer). The controls: another opcode
// never waits there; the queue emptied (a pick's promotion), nothing waits.
func TestALongMessageWaitsUntilItIsAllAcknowledged(t *testing.T) {
	r := newStreamRig(t)
	q, conn := r.sess.Queue(), r.sess.Connection()
	if q.Waits(protocol.OpcodeServerWorldsData) {
		t.Fatal("an empty queue: the recipes wait in it")
	}
	last := r.long(5)
	if !q.Waits(protocol.OpcodeServerWorldsData) || q.Waits(protocol.OpcodeServerWorldsChars) {
		t.Fatalf("queued: waits %v, another opcode %v", q.Waits(protocol.OpcodeServerWorldsData), q.Waits(protocol.OpcodeServerWorldsChars))
	}
	r.sess.TakeServerWorlds()
	NewRDPHandler(r.l).SendWaiting(r.sess)
	r.read()
	if !q.Waits(protocol.OpcodeServerWorldsData) {
		t.Fatal("sent and not acknowledged: it no longer waits")
	}
	conn.SetClientAck(last-1, 0)
	q.RemoveReliables()
	if !q.Waits(protocol.OpcodeServerWorldsData) {
		t.Fatal("all but its last fragment acknowledged: it no longer waits")
	}
	conn.SetClientAck(last, 0)
	q.RemoveReliables()
	if q.Waits(protocol.OpcodeServerWorldsData) {
		t.Fatal("all acknowledged: it still waits")
	}
	r.long(3)
	q.Clear()
	if q.Waits(protocol.OpcodeServerWorldsData) {
		t.Fatal("the queue emptied: it still waits")
	}
}
