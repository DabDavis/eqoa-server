package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// clientDatagram is a client's packet of exactly size bytes: one reliable
// message of opcode whose data fills it, as route_flush makes (a bug report with
// its note comes to this).
func clientDatagram(t *testing.T, size int, instance uint32, opcode uint16) []byte {
	t.Helper()
	body := func(n int) []byte {
		b := make([]byte, 16+n)
		w := buffer.NewWriter(b)
		w.WriteByte(0)     // body flags
		w.WriteUint16(1)   // arrival number
		w.WriteByte(0xFB)  // reliable
		w.WriteSize(n + 2) // the opcode and the data
		w.WriteUint16(7)   // its number
		w.WriteUint16(opcode)
		w.WriteBytes(make([]byte, n))
		return b[:w.Position()]
	}
	for n := size; n > 0; n-- {
		if p := BuildPacketData(0xB991, 0x1234, instance, true, false, body(n)); len(p) == size {
			return p
		}
	}
	t.Fatalf("no data length makes a packet of %d bytes", size)
	return nil
}

// A client's packet reaches its handler whole at any size up to the 1500 of
// DRDP's route_flush. Cut at the old 1328-byte read buffer, those past it failed
// their CRC, so a long bug report was never filed and Support said "Sending
// message" for good (jib, 2026-11-06). The control: a packet whose CRC really is
// wrong, which stays dropped.
func TestAClientsLargestPacketIsReadWhole(t *testing.T) {
	const op = 0x0AA1
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan int, 8)
	reg := NewHandlerRegistry()
	reg.Register(op, func(_ *session.Session, msg *protocol.Message) { got <- len(msg.Data()) })
	l := NewListener(&config.Config{}, session.NewManager(), reg, nil)
	l.conn = conn
	l.wg.Add(1)
	go l.readLoop()
	defer l.Stop()

	send, err := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer send.Close()

	for i, size := range []int{600, MaxPacketSize, MaxPacketSize + 1, 1400, 1500} {
		p := clientDatagram(t, size, 0x1000+uint32(i), op)
		if _, err := send.Write(p); err != nil {
			t.Fatal(err)
		}
		select {
		case n := <-got:
			if n < size-40 {
				t.Errorf("a packet of %d bytes: the handler got %d bytes of data", size, n)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("a packet of %d bytes never reached its handler", size)
		}
	}

	bad := clientDatagram(t, 1400, 0x2000, op)
	bad[len(bad)-1] ^= 0xFF
	if _, err := send.Write(bad); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-got:
		t.Errorf("a packet with a wrong CRC reached its handler (%d bytes)", n)
	case <-time.After(300 * time.Millisecond):
	}
}
