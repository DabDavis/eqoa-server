package udp

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A shutdown tells every client first (F3 20261026-063102: a restart left a
// game in a world that no longer answered, a pet carried on along its last
// word): 0x07C0, CLIENT's Disconnect(17), reaches each session's address at
// once, in the world or before it.
func TestAShutdownTellsTheClients(t *testing.T) {
	server, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	sm := session.NewManager()
	l := NewListener(nil, sm, nil, entity.NewWorld())
	l.conn = server
	var clients []*net.UDPConn
	for i := uint32(1); i <= 2; i++ {
		c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		clients = append(clients, c)
		sm.CreateSession(c.LocalAddr().(*net.UDPAddr), i, i, uint16(i), uint16(i+1), true)
	}

	if n := l.NoticeShutdown(); n != 2 {
		t.Fatalf("told %d clients, want 2", n)
	}
	got := make([]byte, 2048)
	for i, c := range clients {
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := c.ReadFromUDP(got)
		if err != nil {
			t.Fatalf("client %d heard nothing: %v", i, err)
		}
		if !bytes.Contains(got[:n], []byte{0xC0, 0x07}) {
			t.Fatalf("client %d's datagram carries no 0x07C0: %x", i, got[:n])
		}
	}
}
