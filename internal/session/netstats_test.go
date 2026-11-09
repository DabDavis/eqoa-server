package session

import (
	"bytes"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
)

// A flush ack names one of our packets: its round trip is a sample, once;
// the mask names more (bit i: ack-1-i), as connection_record_acks takes them.
// The control: a packet never sent (no time on the ring) gives none.
func TestNetStatsRoundTripsOffFlushAcks(t *testing.T) {
	var st NetStats
	t0 := time.Unix(1_800_000_000, 0)
	for seq := uint16(1); seq <= 10; seq++ {
		st.NoteSent(seq, 100, t0.Add(time.Duration(seq)*10*time.Millisecond))
	}
	// ack 10 at +200 ms: 10 went at +100, so 100 ms; mask bits 0 and 2:
	// packets 9 (at +90: 110 ms) and 7 (at +70: 130 ms)
	st.NoteFlushAck(10, 0b101, true, t0.Add(200*time.Millisecond))
	smooth, least, most := st.RTT()
	if least != 100*time.Millisecond || most != 130*time.Millisecond || smooth == 0 {
		t.Fatalf("round trips: smooth %v least %v most %v", smooth, least, most)
	}
	st.mu.Lock()
	n := st.rttN
	st.mu.Unlock()
	if n != 3 {
		t.Fatalf("%d samples, want 3", n)
	}
	// the same ack again: no sample (each packet's time is taken once)
	st.NoteFlushAck(10, 0b101, true, t0.Add(time.Second))
	// a packet never sent: none
	st.NoteFlushAck(40, 0, false, t0.Add(time.Second))
	st.mu.Lock()
	n = st.rttN
	st.mu.Unlock()
	if n != 3 {
		t.Fatalf("%d samples after acks of nothing new, want 3", n)
	}
}

// The mask says which of our last packets the client never had: 64 when
// the connection has sent that many, fewer at its start.
func TestNetStatsLostOfOurLastPackets(t *testing.T) {
	var st NetStats
	now := time.Now()
	st.NoteFlushAck(200, ^uint64(0)&^(1<<3|1<<40), true, now) // all but two
	st.mu.Lock()
	lost, of := st.lostOfLastLocked()
	st.mu.Unlock()
	if lost != 2 || of != 64 {
		t.Fatalf("lost %d of %d, want 2 of 64", lost, of)
	}
	var early NetStats
	early.NoteFlushAck(5, 0b1011, true, now) // packets 4, 3, 1 had; 2 not
	early.mu.Lock()
	lost, of = early.lostOfLastLocked()
	early.mu.Unlock()
	if lost != 1 || of != 4 {
		t.Fatalf("early: lost %d of %d, want 1 of 4", lost, of)
	}
	var none NetStats
	none.NoteFlushAck(5, 0, false, now)
	none.mu.Lock()
	lost, _ = none.lostOfLastLocked()
	none.mu.Unlock()
	if lost != -1 {
		t.Fatalf("no mask sent: %d, want -1 (no word)", lost)
	}
}

// netSession is a session with a connection, for the queue's own stats.
func netSession(t *testing.T) *Session {
	t.Helper()
	s := NewSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 4321}, 7, 7, 1, 2, true)
	s.SetCharacterName("Netty")
	return s
}

// queued is a reliable message queued on s, as a handler queues one.
func queued(s *Session, op protocol.GameOpcode) *protocol.Message {
	m := protocol.NewMessage(protocol.MessageTypeReliable, op)
	m.SetData([]byte{byte(op), byte(op >> 8), 1, 2, 3})
	s.Queue().AddWithSequence(m)
	return m
}

// The report counts what went out and waits (never what is queued unsent),
// its oldest by when it was queued, and the client's ack; the "Net:" line
// names the session and comes at most every netLogEvery while it resends,
// where the line it replaced came every packet.
func TestNetLineNamesTheSessionAndIsRateLimited(t *testing.T) {
	s := netSession(t)
	q := s.Queue()
	for i := 0; i < 6; i++ {
		queued(s, protocol.OpcodeServerReady)
	}
	// four went out half a second ago; two never left
	sent := q.GetPending()
	past := time.Now().Add(-1500 * time.Millisecond)
	for _, m := range sent[:4] {
		m.LastSent = past
		m.Queued = past
	}
	q.RequeueBatch(sent[4:])
	s.Connection().Net().NoteAck(0, time.Now())

	r := q.Report(time.Now())
	if r.Unacked != 4 || r.Pending != 2 || r.FirstSeq != 1 || r.LastSeq != 4 || r.Oldest < time.Second {
		t.Fatalf("report: %+v", r)
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	for i := 0; i < 3; i++ {
		if got := q.TakeRetransmits(100*time.Millisecond, 4); len(got) == 0 && i == 0 {
			t.Fatal("nothing taken to resend")
		}
		for _, m := range q.reliable {
			m.LastSent = past // due again at once
		}
	}
	out := buf.String()
	if n := strings.Count(out, "Net: "); n != 1 {
		t.Fatalf("%d Net: lines for three packets of resends within %v, want 1:\n%s", n, netLogEvery, out)
	}
	if !strings.Contains(out, "203.0.113.7:4321 'Netty' resent 4 reliable message(s) in 1 packet(s)") ||
		!strings.Contains(out, "unacked") || !strings.Contains(out, "client acked 0") {
		t.Fatalf("the Net: line: %s", out)
	}
	if strings.Contains(out, "Queue: retransmitting") {
		t.Fatalf("the old line a packet is still written: %s", out)
	}
	if r := q.Report(time.Now()); r.ResentMinute != 12 || r.ResentPktsMin != 3 {
		t.Fatalf("resends of the last minute: %d in %d packets, want 12 in 3", r.ResentMinute, r.ResentPktsMin)
	}
}

// An acknowledged reliable message is not handed back to the message pool:
// a resend of it can still sit in the queue (put back when its packet was
// full), and the pool hands a message out again at once, to any session, its
// number, type and bytes written over under that reference. It is marked
// acknowledged instead, so the queued copy is not sent. The control: a
// message the pool hands out is a fresh one.
func TestAnAckedMessageIsNotPooledUnderAQueuedCopy(t *testing.T) {
	s := netSession(t)
	q := s.Queue()
	m := queued(s, protocol.OpcodeServerReady) // seq 1, from the pool
	q.GetPending()
	// resent, and put back when its packet was full
	m.LastSent = time.Now().Add(-time.Second)
	again := q.TakeRetransmits(100*time.Millisecond, 4)
	if len(again) != 1 || again[0] != m {
		t.Fatalf("resend: %v", again)
	}
	q.RequeueBatch(again)

	s.Connection().SetClientLastReceivedMessage(1)
	q.RemoveReliables()
	if !m.Acked {
		t.Fatal("the acknowledged message is not marked so")
	}
	for i := 0; i < 64; i++ {
		if got := protocol.GetMessage(); got == m {
			t.Fatalf("the pool handed out the acknowledged message still queued (after %d gets)", i)
		}
	}
	if q.PendingCount() != 1 || q.PeekPending()[0] != m || m.Sequence != 1 {
		t.Fatalf("the queued copy changed: %d pending, seq %d", q.PendingCount(), m.Sequence)
	}
}
