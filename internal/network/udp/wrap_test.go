package udp

import (
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A long session passes 65535 in every count: the client's packet numbers
// after two to three hours (eqoaclassic, 01:39:15 UTC 2026-11-09: arrival 0
// to 458 dropped against 65535, the session's acks never taken again), our
// message numbers after 65,535 reliable messages. Each is compared by the
// signed 16-bit difference, as connection.cpp compares them.
func TestPacketAndMessageNumbersPass65535(t *testing.T) {
	sess := session.NewSession(nil, 9, 9, 1, 2, true)
	conn := sess.Connection()
	r := NewRDPHandler(nil)
	packet := func(arrival, gAck uint16) *Packet {
		return &Packet{Header: &protocol.SegmentHeader{Flags: protocol.SegmentFlagIsRemote}, Body: &protocol.SegmentBody{
			Flags:         protocol.BodyFlagFlushAck | protocol.BodyFlagGuaranteedAck,
			ArrivalNumber: arrival, FlushAck: 7, GuaranteedAck: gAck,
		}}
	}

	// a long session's counts, walked up to the wrap
	for _, n := range []uint16{20000, 40000, 60000} {
		r.ProcessIncoming(sess, packet(n, n-10))
	}
	// the client's packets: 65535, then 0 and 1 after the wrap, are each
	// newer; 65535 again is old. The control: an old one is still dropped.
	r.ProcessIncoming(sess, packet(65535, 65530))
	if got := conn.LastReceivedPacketSeq(); got != 65535 {
		t.Fatalf("arrival 65535: newest %d", got)
	}
	r.ProcessIncoming(sess, packet(0, 65531))
	if got := conn.LastReceivedPacketSeq(); got != 0 {
		t.Fatalf("arrival 0 after 65535: newest %d, want 0", got)
	}
	r.ProcessIncoming(sess, packet(1, 65532))
	r.ProcessIncoming(sess, packet(65535, 65533))
	if got := conn.LastReceivedPacketSeq(); got != 1 {
		t.Fatalf("an old packet (65535 after 1) was taken: newest %d", got)
	}
	if got := conn.ClientLastReceivedMessage(); got != 65532 {
		t.Fatalf("the acks: %d, want 65532 (the old packet's 65533 not taken)", got)
	}

	// in a packet our messages go 65535 before 0 (RemoveReliables' side is
	// session's TestRemoveReliablesPasses65535)
	merged := mergeReliablesInSequenceOrder(nil, []*protocol.Message{
		{Type: protocol.MessageTypeReliable, Sequence: 0}, {Type: protocol.MessageTypeReliable, Sequence: 65535}})
	if merged[0].Sequence != 65535 || merged[1].Sequence != 0 {
		t.Fatalf("merged: %v", seqs(merged))
	}
	// the client's ack past the wrap moves on
	conn.SetClientLastReceivedMessage(1)
	if got := conn.ClientLastReceivedMessage(); got != 1 {
		t.Fatalf("the client's ack of 1 after 65535: %d", got)
	}
}

// A reliable message numbered 0 by AddWithSequence (the count past 65535)
// goes out as 0: numbered again in PreparePacket it was a number the client
// waited for that no list would resend.
func TestAReliableMessageNumberedZeroKeepsItsNumber(t *testing.T) {
	sess := session.NewSession(nil, 9, 9, 1, 2, true)
	for i := 0; i < 65535; i++ {
		sess.Connection().NextMessageSeq()
	}
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
	m.SetData([]byte{0xE0, 0x07})
	if seq := sess.Queue().AddWithSequence(m); seq != 0 {
		t.Fatalf("the 65,536th message numbered %d, want 0", seq)
	}
	r := NewRDPHandler(nil)
	if data := r.PreparePacket(sess); data == nil {
		t.Fatal("no packet")
	}
	if m.Sequence != 0 {
		t.Fatalf("the message went out numbered %d, want 0", m.Sequence)
	}
	if got := sess.Connection().CurrentMessageSeq(); got != 0 {
		t.Fatalf("the count moved on to %d: the message was numbered again", got)
	}
}
