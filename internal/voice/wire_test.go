package voice

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pion/turn/v4"
)

// coturn's REST credential, against a vector made with Python's hmac (a
// made-up secret); the control: another secret gives another.
func TestCredentialIsCoturns(t *testing.T) {
	if got := Credential("a-test-secret-only", "1700000000:v0000002a"); got != "fL+E108VuYhsBuWvBPKRHO+mbiA=" {
		t.Fatalf("credential %q", got)
	}
	if got := Credential("another-secret", "1700000000:v0000002a"); got != "cFEbMLOsDCLnJBmBOQ0Zc24eseI=" {
		t.Fatalf("another secret: %q", got)
	}
	if got := Username(time.Unix(1700000000, 0), 42); got != "1700000000:v0000002a" {
		t.Fatalf("username %q", got)
	}
}

// A long message is split on whole characters and put back whole, its
// parts in any order; a repeat, a kind that changes and a count that
// changes are no message.
func TestSplitAndAssemble(t *testing.T) {
	text := strings.Repeat("é", 1000) + strings.Repeat("a", 7) // 2,007 bytes
	parts := Split(text)
	if len(parts) != 3 {
		t.Fatalf("%d parts", len(parts))
	}
	for _, p := range parts {
		if len(p) > MaxChunk || !strings.HasPrefix(p, "é") && !strings.HasPrefix(p, "a") {
			t.Fatalf("a part of %d bytes starting %q", len(p), p[:2])
		}
	}
	now := time.Now()
	var a Assembler
	order := []int{2, 0, 1}
	for i, k := range order {
		got, done := a.Add(7, KindOffer, k, len(parts), parts[k], now)
		if done != (i == len(order)-1) {
			t.Fatalf("part %d: done %v", k, done)
		}
		if done && got != text {
			t.Fatal("reassembled wrong")
		}
	}
	if got := Split(""); len(got) != 1 || got[0] != "" {
		t.Fatalf("an empty text: %q", got)
	}
	if _, done := a.Add(8, KindOffer, 0, 2, "x", now); done {
		t.Fatal("half a message")
	}
	if _, done := a.Add(8, KindOffer, 0, 2, "x", now); done {
		t.Fatal("a repeat finished it")
	}
	if _, done := a.Add(8, KindCandidate, 1, 2, "y", now); done {
		t.Fatal("a part of another kind finished it")
	}
	if _, done := a.Add(9, KindOffer, 0, 2, "x", now); done {
		t.Fatal(".")
	}
	if _, done := a.Add(9, KindOffer, 1, 3, "y", now); done {
		t.Fatal("a part of another count finished it")
	}
	for _, c := range []struct {
		kind        byte
		part, parts int
		chunk       string
	}{{KindAnswer, 0, 1, ""}, {5, 0, 1, ""}, {KindOffer, 1, 1, ""}, {KindOffer, 0, 65, ""}, {KindOffer, 0, 1, strings.Repeat("x", 901)}} {
		if CheckPart(c.kind, c.part, c.parts, c.chunk) == nil {
			t.Errorf("%+v passed", c)
		}
	}
	if CheckPart(KindCandidate, 63, 64, strings.Repeat("x", 900)) != nil {
		t.Error("the largest part refused")
	}
}

// The allowance: 50 messages in a second, the 51st refused, refilled a
// second later; 64 KB in a second.
func TestRate(t *testing.T) {
	var r Rate
	now := time.Now()
	for i := 0; i < RateMsgs; i++ {
		if !r.Take(10, now) {
			t.Fatalf("message %d refused", i+1)
		}
	}
	if r.Take(10, now) {
		t.Fatal("the 51st passed")
	}
	if !r.Take(10, now.Add(time.Second)) {
		t.Fatal("not refilled")
	}
	var b Rate
	if !b.Take(RateBytes-10, now) || b.Take(20, now) {
		t.Fatal("bytes")
	}
}

func TestChannelID(t *testing.T) {
	if id, ok := ChannelID("d:", "d:0123456789ABCDEF0123456789abcdef"); !ok || id != "d:0123456789abcdef0123456789abcdef" {
		t.Fatalf("%q %v", id, ok)
	}
	for _, bad := range []string{"d:0123", "r:0123456789abcdef0123456789abcdef", "d:0123456789abcdef0123456789abcdeg", "d:0123456789abcdef0123456789abcdef0"} {
		if _, ok := ChannelID("d:", bad); ok {
			t.Errorf("%q passed", bad)
		}
	}
}

// The embedded TURN server: a minted credential allocates, a wrong one and
// an expired one do not; it relays to the forwarding server only (a
// permission for another address is refused, the server's own granted).
func TestTURN(t *testing.T) {
	s, err := New(Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1",
		TURN: TURNConfig{Port: 0, Secret: "a-test-secret-only", RelayMinPort: 40000, RelayMaxPort: 40100}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if s.turn != nil {
		t.Fatal("port 0 is no TURN server")
	}
	ts, err := startTURN(TURNConfig{Secret: "a-test-secret-only", RelayMinPort: 40000, RelayMaxPort: 40100}, "127.0.0.1", net.ParseIP("127.0.0.1").To4())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.close)
	addr := strings.TrimSuffix(strings.TrimPrefix(ts.urls[0], "turn:"), "?transport=udp")
	alloc := func(user, pass string) (*turn.Client, net.PacketConn, error) {
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		c, err := turn.NewClient(&turn.ClientConfig{TURNServerAddr: addr, STUNServerAddr: addr, Conn: conn,
			Username: user, Password: pass, Realm: "eqoa", RTO: 200 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close(); conn.Close() })
		if err := c.Listen(); err != nil {
			t.Fatal(err)
		}
		relay, err := c.Allocate()
		return c, relay, err
	}
	user := Username(time.Now().Add(time.Hour), 7)
	_, relay, err := alloc(user, Credential("a-test-secret-only", user))
	if err != nil {
		t.Fatalf("a minted credential: %v", err)
	}
	if _, _, err := alloc(user, Credential("another-secret", user)); err == nil {
		t.Fatal("a wrong credential allocated")
	}
	old := Username(time.Now().Add(-time.Minute), 7)
	if _, _, err := alloc(old, Credential("a-test-secret-only", old)); err == nil {
		t.Fatal("an expired credential allocated")
	}
	// another address: refused (the write's permission fails); the server's
	// own address (a UDP socket standing in for the media port): granted
	if _, err := relay.WriteTo([]byte("x"), &net.UDPAddr{IP: net.ParseIP("127.0.0.2"), Port: 9}); err == nil {
		t.Fatal("a permission to another address was granted")
	}
	sfu, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer sfu.Close()
	if _, err := relay.WriteTo([]byte("hello"), sfu.LocalAddr()); err != nil {
		t.Fatalf("to the forwarding server's address: %v", err)
	}
	buf := make([]byte, 64)
	sfu.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := sfu.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Fatalf("relayed: %q %v", buf[:n], err)
	}
}
