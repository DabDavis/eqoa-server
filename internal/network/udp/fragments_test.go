package udp

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// messageDatagrams is message whole (its opcode first) as a client's DRDP
// sends it from number seq: connection_send_guaranteed's pieces of 1,300
// bytes, 0xFA but the last (0xFB), one a datagram as route_flush has room for,
// each its own arrival number from arrival.
func messageDatagrams(t *testing.T, instance uint32, seq, arrival uint16, whole []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for off := 0; off == 0 || off < len(whole); off += 1300 {
		end := min(off+1300, len(whole))
		typ := byte(0xFA)
		if end == len(whole) {
			typ = 0xFB
		}
		b := make([]byte, 16+end-off)
		w := buffer.NewWriter(b)
		w.WriteByte(0) // body flags
		w.WriteUint16(arrival)
		w.WriteByte(typ)
		w.WriteSize(end - off)
		w.WriteUint16(seq)
		w.WriteBytes(whole[off:end])
		out = append(out, BuildPacketData(0xB991, 0x1234, instance, true, false, b[:w.Position()]))
		seq++
		arrival++
	}
	return out
}

// A petition of 2,047 characters reaches its handler whole, once, though it
// comes in four datagrams (4,103 bytes: three pieces and a last); none of its
// pieces reaches a handler of its own. Ten of them go back to back between
// short messages, so their packets' goroutines run at once (turns.go). The text
// is U+EEEE, so each later piece starts with bytes that read as opcode
// 0xEEEE: the server before this read every piece as a message, the first cut
// short to its handler and the rest to 0xEEEE's. The control: a petition of 511
// characters (1,031 bytes), one datagram, goes as it always went.
func TestALongMessageReachesItsHandlerWhole(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var petitions [][]byte
	var notes, garbage int
	got := make(chan struct{}, 64)
	reg := NewHandlerRegistry()
	reg.Register(0x0AA1, func(_ *session.Session, msg *protocol.Message) {
		mu.Lock()
		petitions = append(petitions, append([]byte{0xA1, 0x0A}, msg.Data()...))
		mu.Unlock()
		got <- struct{}{}
	})
	reg.Register(0x7E0F, func(*session.Session, *protocol.Message) {
		mu.Lock()
		notes++
		mu.Unlock()
		got <- struct{}{}
	})
	reg.Register(0xEEEE, func(*session.Session, *protocol.Message) {
		mu.Lock()
		garbage++
		mu.Unlock()
		got <- struct{}{}
	})
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

	petition := func(n int) []byte {
		b := []byte{0xA1, 0x0A, 0, 2, 24}
		b = binary.LittleEndian.AppendUint32(b, uint32(n))
		for i := 0; i < n; i++ {
			b = binary.LittleEndian.AppendUint16(b, 0xEEEE)
		}
		return b
	}
	long, short := petition(2047), petition(511)
	if len(long) != 4103 || len(short) != 1031 {
		t.Fatalf("a petition of 2,047 is %d bytes, one of 511 %d", len(long), len(short))
	}
	const instance = 0x5150
	seq, arrival := uint16(1), uint16(1)
	var datagrams [][]byte
	add := func(whole []byte) {
		d := messageDatagrams(t, instance, seq, arrival, whole)
		datagrams = append(datagrams, d...)
		seq += uint16(len(d))
		arrival += uint16(len(d))
	}
	const rounds = 10
	for i := 0; i < rounds; i++ {
		add([]byte{0x0F, 0x7E, 'h', 'i'}) // a note, whole
		add(long)
	}
	add(short)
	for _, d := range datagrams {
		if len(d) > MaxReceiveSize {
			t.Fatalf("a datagram of %d bytes", len(d))
		}
		if _, err := send.Write(d); err != nil {
			t.Fatal(err)
		}
	}
	for want := 2*rounds + 1; want > 0; want-- {
		select {
		case <-got:
		case <-time.After(5 * time.Second):
			mu.Lock()
			t.Fatalf("%d petitions and %d short messages handled, %d to come", len(petitions), notes, want)
			mu.Unlock()
		}
	}
	time.Sleep(100 * time.Millisecond) // anything handled past what was sent
	mu.Lock()
	defer mu.Unlock()
	if garbage != 0 || notes != rounds || len(petitions) != rounds+1 {
		t.Fatalf("%d petitions, %d short messages, %d pieces taken for messages", len(petitions), notes, garbage)
	}
	var whole, controls int
	for _, p := range petitions {
		switch {
		case bytes.Equal(p, long):
			whole++
		case bytes.Equal(p, short):
			controls++
		default:
			t.Errorf("a petition of %d bytes reached its handler", len(p))
		}
	}
	if whole != rounds || controls != 1 {
		t.Fatalf("%d long petitions whole, %d short", whole, controls)
	}
}

// A packet waits for those read before it from its address only when asked
// (waitEarlier), and then for all of them: one read two before it still
// being handled holds it though the one just before is done. A stuck one
// holds it turnWait at most.
func TestAPacketWaitsForThoseReadBeforeIt(t *testing.T) {
	l := &Listener{}
	a := netip.MustParseAddrPort("127.0.0.1:4000")
	now := time.Now()
	first, second, third := l.nextTurn(a, now), l.nextTurn(a, now), l.nextTurn(a, now)
	other := l.nextTurn(netip.MustParseAddrPort("127.0.0.1:4001"), now)
	other.waitEarlier() // another address's first packet waits for nothing
	go second.finish()  // done itself, but not before first is
	waited := make(chan struct{})
	go func() {
		third.waitEarlier()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("the third packet did not wait for the first")
	case <-time.After(50 * time.Millisecond):
	}
	first.finish()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("the third packet went on waiting once the first two were done")
	}

	// stuck: the next waits turnWait, then goes on
	stuck, next := l.nextTurn(a, now), l.nextTurn(a, now)
	_ = stuck
	start := time.Now()
	next.waitEarlier()
	if d := time.Since(start); d < turnWait-50*time.Millisecond || d > turnWait+500*time.Millisecond {
		t.Fatalf("waited %v for a stuck packet", d)
	}
}
