package udp

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// slotsTick is a session in the world whose client keeps slots entity slots,
// an entity on every one of them past the player's, and the socket its
// datagrams reach.
func slotsTick(t *testing.T, slots int) (*RDPHandler, *session.Session, *net.UDPConn) {
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
	sess := session.NewSession(cli.LocalAddr().(*net.UDPAddr), 1, 1, 1, 2, false)
	sess.SetCharacterInWorld(true)
	conn := sess.Connection()
	conn.SetEntitySlots(slots)
	for slot := 1; slot < slots; slot++ {
		v := make([]byte, 162)
		v[0], v[5] = 1, 0x82 // flag, a creature
		binary.LittleEndian.PutUint32(v[1:], uint32(100000+slot))
		conn.GetServerObject(slot).SetData(v)
	}
	return NewRDPHandler(&Listener{conn: srv}), sess, cli
}

// tick runs one TickSession and returns the entity channels each datagram
// carried.
func tick(t *testing.T, r *RDPHandler, sess *session.Session, cli *net.UDPConn) [][]byte {
	t.Helper()
	r.TickSession(sess)
	var out [][]byte
	buf := make([]byte, 65536)
	for {
		cli.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, _, err := cli.ReadFromUDP(buf)
		if err != nil {
			return out
		}
		p, err := ParsePacket(append([]byte(nil), buf[:n]...))
		if err != nil {
			t.Fatalf("datagram %d does not parse: %v", len(out), err)
		}
		var chs []byte
		for _, m := range p.Body.Messages {
			if session.EntitySlot(byte(m.Type)) >= 1 {
				chs = append(chs, byte(m.Type))
				if got := binary.LittleEndian.Uint32(m.Data()[1:]); got != uint32(100000+session.EntitySlot(byte(m.Type))) {
					t.Fatalf("channel %#x carries entity %d", m.Type, got)
				}
			}
		}
		out = append(out, chs)
	}
}

// A client that never asked for more slots (a PS2) is sent what it always
// was: one datagram a tick, at most MaxStateUpdatesPerTick entity channels in
// it, all of them 0x01-0x17.
func TestPS2TickIsOneDatagram(t *testing.T) {
	r, sess, cli := slotsTick(t, session.RetailEntitySlots)
	seen := map[byte]bool{}
	for k := 0; k < 3; k++ {
		got := tick(t, r, sess, cli)
		if len(got) != 1 || len(got[0]) > MaxStateUpdatesPerTick {
			t.Fatalf("tick %d: %d datagrams %v, want one with at most %d entity channels", k, len(got), got, MaxStateUpdatesPerTick)
		}
		for _, ch := range got[0] {
			if ch > 0x17 {
				t.Fatalf("tick %d: channel %#x, which a PS2 does not read", k, ch)
			}
			seen[ch] = true
		}
	}
	if len(seen) != 23 {
		t.Fatalf("3 ticks served %d of the 23 channels", len(seen))
	}
}

// A client keeping 128 has a datagram more for each 24 slots, each of them as
// full as a PS2's, on the slots' own channels, and every channel is served as
// soon as a PS2's would be: 127 in three ticks.
func TestExtendedTickServesEverySlot(t *testing.T) {
	r, sess, cli := slotsTick(t, 128)
	seen := map[byte]int{}
	for k := 0; k < 3; k++ {
		got := tick(t, r, sess, cli)
		if len(got) > statePacketsPerTick(128) {
			t.Fatalf("tick %d: %d datagrams, want at most %d", k, len(got), statePacketsPerTick(128))
		}
		for i, chs := range got {
			if len(chs) > MaxStateUpdatesPerTick {
				t.Fatalf("tick %d datagram %d: %d entity channels, more than one fits", k, i, len(chs))
			}
			for _, ch := range chs {
				if ch >= 0x40 && ch < 0x6C {
					t.Fatalf("tick %d: an entity on channel %#x, which CLIENT reads as something else", k, ch)
				}
				seen[ch]++
			}
		}
	}
	if len(seen) != 127 {
		t.Fatalf("3 ticks served %d of the 127 channels", len(seen))
	}
	for ch, n := range seen {
		if n != 1 {
			t.Fatalf("channel %#x served %d times for one change", ch, n)
		}
	}
	if statePacketsPerTick(24) != 1 || statePacketsPerTick(25) != 2 || statePacketsPerTick(204) != 9 {
		t.Fatal("statePacketsPerTick is not one datagram for each 24 slots")
	}
}

// An ack names a channel; it reaches the slot that channel carries, so the
// next change goes out against it (refnum 1), not whole. Slot 64 is channel
// 0x6C: an ack on channel 0x40 (64, the group's) leaves it alone.
func TestStateAckOnAnExtendedChannel(t *testing.T) {
	r, sess, cli := slotsTick(t, 100)
	for k := 0; k < 3; k++ {
		tick(t, r, sess, cli)
	}
	state := sess.Connection().GetServerObject(64)
	seq := state.GetSeqNum() - 1
	change := func() byte {
		v := state.GetData()
		v[10] ^= 0xFF
		state.SetData(v)
		_, _, ref := state.GenerateXORDelta()
		return ref
	}
	ack := func(ch byte, seq uint16) {
		r.processStateAcks(sess, &protocol.SegmentBody{
			Flags:     protocol.BodyFlagClientUpdateAck,
			StateAcks: []protocol.StateAck{{Channel: ch, Ack: seq}},
		})
	}
	ack(0x40, seq)
	if ref := change(); ref != 0 {
		t.Fatalf("an ack on the group channel 0x40 moved slot 64: refnum %d, want 0 (whole)", ref)
	}
	ack(0x6C, seq+1)
	if ref := change(); ref != 1 {
		t.Fatalf("the change after an ack on 0x6C has refnum %d, want 1 (against the ack)", ref)
	}
}
