package udp

import (
	"encoding/hex"
	"net"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// clientSegment is a client's packet as CLIENT's connection_flush builds it:
// the drdp ports, the segment header (flags with the body's size; the
// instance; a dst when it names a software endpoint, flag 0x800), the body
// (flags 0x23: an arrival number, a flush ack, a guaranteed ack) and the CRC.
func clientSegment(flags uint64, instance uint32, dst uint64, arrival, flushAck, gAck uint16) []byte {
	body := []byte{0x23, byte(arrival), byte(arrival >> 8), byte(flushAck), byte(flushAck >> 8), byte(gAck), byte(gAck >> 8)}
	out := []byte{0x95, 0x45, 0x91, 0xB9}
	out = append(out, protocol.Encode7BitUInt64(flags|uint64(len(body)))...)
	out = append(out, byte(instance), byte(instance>>8), byte(instance>>16), byte(instance>>24))
	if flags&uint64(protocol.SegmentFlagIsRemote) != 0 {
		out = append(out, protocol.Encode7BitUInt64(dst)...)
	}
	return protocol.AppendCRC(append(out, body...))
}

// After the promotion the client's lobby connection still sends: here its
// acks of the live look's character list, which came after it sent the pick
// (the rig's failing login, net1109 j3: arrival 4, flush ack 2, guaranteed
// ack 8 on the same instance). They must not become the promoted
// connection's numbers: the lobby's arrival as our flush ack is what the
// client's new connection refuses whole, and the lobby's guaranteed ack
// would retire promoted messages of those numbers unsent. The promoted
// connection's own packet (flag 0x800, dst the SessionID) is taken. The
// control: the very same lobby packet before the promotion is taken.
func TestPromotedSessionLeavesItsLobbyConnectionsPacketsAlone(t *testing.T) {
	const instance = 0x3EA84596
	sm := session.NewManager()
	l := NewListener(nil, sm, nil, nil)
	sock, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sock.Close() })
	l.conn = sock
	sink, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sink.Close() })
	addr := sink.LocalAddr().(*net.UDPAddr)
	sess := sm.CreateSession(addr, instance, instance, 0x4595, 0xB991, false)

	// the very bytes of the rig's lobby packet: 0x7000, no dst
	var lobby []byte
	lobby, err = hex.DecodeString("954591b987e0019645a83e2304000200080041bbb3f6")
	if err != nil {
		t.Fatal(err)
	}
	if got := clientSegment(0x7000, instance, 0, 4, 2, 8); hex.EncodeToString(got) != hex.EncodeToString(lobby) {
		t.Fatalf("the builder: %x, the rig's packet %x", got, lobby)
	}

	// before the promotion: the lobby's own session takes it
	l.handlePacket(lobby, addr, nil)
	if got := sess.Connection().LastReceivedPacketSeq(); got != 4 {
		t.Fatalf("the lobby's packet before the promotion: arrival %d taken, want 4", got)
	}

	sm.PromoteSessionToMaster(sess, 0x1DCD650C)
	for i := 0; i < 5; i++ {
		m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerReady)
		m.SetData([]byte{0xE0, 0x07})
		sess.Queue().AddWithSequence(m)
	}
	late := clientSegment(0x7000, instance, 0, 5, 3, 9)
	l.handlePacket(late, addr, nil)
	conn := sess.Connection()
	if got := conn.LastReceivedPacketSeq(); got != 0 {
		t.Fatalf("the lobby's late packet set the promoted connection's arrival to %d: our next flush ack would name a packet its new connection never sent", got)
	}
	if got := conn.ClientLastReceivedMessage(); got != 0 {
		t.Fatalf("the lobby's late packet acked the promoted stream to %d", got)
	}
	if got := sess.Queue().ReliableCount(); got != 5 {
		t.Fatalf("%d of the promoted stream's 5 messages left: the lobby's ack retired them unsent", got)
	}

	// the promoted connection's own: 0x3800, dst the SessionID
	own := clientSegment(0x3800, instance, 0x1DCD650C, 1, 1, 2)
	l.handlePacket(own, addr, nil)
	if got := conn.LastReceivedPacketSeq(); got != 1 {
		t.Fatalf("the promoted connection's packet: arrival %d, want 1", got)
	}
	if got := conn.ClientLastReceivedMessage(); got != 2 {
		t.Fatalf("the promoted connection's ack: %d, want 2", got)
	}
}
