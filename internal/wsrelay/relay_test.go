package wsrelay

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// udpPeer is a UDP socket on loopback that counts what it receives and, if
// echo is set, sends each datagram back to its sender.
type udpPeer struct {
	conn     *net.UDPConn
	got      atomic.Int64
	lastFrom atomic.Value // netip.AddrPort
}

func newUDPPeer(t *testing.T, echo bool) *udpPeer {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	p := &udpPeer{conn: c}
	t.Cleanup(func() { c.Close() })
	go func() {
		buf := make([]byte, 65536)
		for {
			n, from, err := c.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			p.got.Add(1)
			p.lastFrom.Store(from)
			if echo {
				c.WriteToUDPAddrPort(buf[:n], from)
			}
		}
	}()
	return p
}

func (p *udpPeer) addr() netip.AddrPort {
	return p.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

// tcpServer accepts on loopback and runs serve on each connection.
func tcpServer(t *testing.T, serve func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				serve(c)
			}()
		}
	}()
	return ln
}

func echoServe(c net.Conn) { io.Copy(c, c) }

func lnPort(ln net.Listener) int { return ln.Addr().(*net.TCPAddr).Port }

func allow(t *testing.T, specs ...string) AllowList {
	t.Helper()
	a, err := ParseAllow(specs)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// logSink collects the relay's log lines; goroutines may log after a test
// has finished, so it never calls t.Logf.
type logSink struct {
	mu    sync.Mutex
	lines []string
}

func (l *logSink) logf(format string, args ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
	l.mu.Unlock()
}

func (l *logSink) has(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.lines {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func newRelay(t *testing.T, cfg Config) (*Relay, *httptest.Server, *logSink) {
	t.Helper()
	logs := &logSink{}
	cfg.Logf = logs.logf
	r := New(cfg)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return r, srv, logs
}

func dial(t *testing.T, srv *httptest.Server, path string, h http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+path, &websocket.DialOptions{HTTPHeader: h})
	if err == nil {
		c.SetReadLimit(1 << 21)
		t.Cleanup(func() { c.CloseNow() })
	}
	return c, resp, err
}

func dialOK(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	c, _, err := dial(t, srv, path, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", path, err)
	}
	return c
}

func read(t *testing.T, c *websocket.Conn, within time.Duration) (websocket.MessageType, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	return c.Read(ctx)
}

func readText(t *testing.T, c *websocket.Conn) string {
	t.Helper()
	typ, m, err := read(t, c, 5*time.Second)
	if err != nil || typ != websocket.MessageText {
		t.Fatalf("want a text message, got %v %q %v", typ, m, err)
	}
	return string(m)
}

// okPort checks an "ok ADDR PORT" greeting and returns the port.
func okPort(t *testing.T, msg, wantAddr string) int {
	t.Helper()
	f := strings.Fields(msg)
	if len(f) != 3 || f[0] != "ok" || f[1] != wantAddr {
		t.Fatalf("greeting %q, want \"ok %s PORT\"", msg, wantAddr)
	}
	p, err := strconv.Atoi(f[2])
	if err != nil || p <= 0 || p > 65535 {
		t.Fatalf("greeting %q: bad port", msg)
	}
	return p
}

func dgram(to netip.AddrPort, payload string) []byte {
	return append(header(to), payload...)
}

func send(t *testing.T, c *websocket.Conn, typ websocket.MessageType, m []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, typ, m); err != nil {
		t.Fatal(err)
	}
}

func TestUDPRoundTrip(t *testing.T) {
	echo := newUDPPeer(t, true)
	other := newUDPPeer(t, false)
	_, srv, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String())})
	c := dialOK(t, srv, "/udp")
	port := okPort(t, readText(t, c), "127.0.0.1")

	send(t, c, websocket.MessageBinary, dgram(echo.addr(), "hello"))
	typ, m, err := read(t, c, 5*time.Second)
	if err != nil || typ != websocket.MessageBinary {
		t.Fatalf("no echo: %v %v", typ, err)
	}
	if !bytes.Equal(m[:6], header(echo.addr())) || string(m[6:]) != "hello" {
		t.Fatalf("echo %x, want header %x + hello", m, header(echo.addr()))
	}
	// the greeting named the socket the datagram really left from
	if from := echo.lastFrom.Load().(netip.AddrPort); int(from.Port()) != port {
		t.Fatalf("datagram left from port %d, greeting said %d", from.Port(), port)
	}
	// header layout: address first octet first, port big-endian
	if h := header(netip.MustParseAddrPort("203.0.113.7:10070")); !bytes.Equal(h, []byte{203, 0, 113, 7, 0x27, 0x56}) {
		t.Fatalf("header %x", h)
	}

	// a port that is not allowed gets nothing, though it is listening
	send(t, c, websocket.MessageBinary, dgram(other.addr(), "nope"))
	// and a datagram FROM it is not handed to the page
	if _, err := other.conn.WriteToUDPAddrPort([]byte("intruder"), netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	send(t, c, websocket.MessageBinary, dgram(echo.addr(), "after"))
	_, m, err = read(t, c, 5*time.Second)
	if err != nil || string(m[6:]) != "after" {
		t.Fatalf("after the refusals got %q, %v (want the echo of \"after\", not the intruder)", m, err)
	}
	if n := other.got.Load(); n != 0 {
		t.Fatalf("the port that is not allowed received %d datagrams", n)
	}
	// negative control: the same peer IS reached once allowed
	_, srv2, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String(), other.addr().String())})
	c2 := dialOK(t, srv2, "/udp")
	readText(t, c2)
	send(t, c2, websocket.MessageBinary, dgram(other.addr(), "yes"))
	deadline := time.Now().Add(2 * time.Second)
	for other.got.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if other.got.Load() != 1 {
		t.Fatalf("an allowed peer received %d datagrams, want 1", other.got.Load())
	}
}

func TestTCPRoundTrip(t *testing.T) {
	ln := tcpServer(t, echoServe)
	_, srv, logs := newRelay(t, Config{AllowTCP: allow(t, fmt.Sprintf("127.0.0.1:%d", lnPort(ln)))})
	c := dialOK(t, srv, fmt.Sprintf("/tcp?host=127.0.0.1&port=%d", lnPort(ln)))
	okPort(t, readText(t, c), "127.0.0.1")

	want := []byte("abc")
	big := make([]byte, 300_000)
	for i := range big {
		big[i] = byte(i * 7)
	}
	want = append(want, big...)
	send(t, c, websocket.MessageBinary, []byte("abc"))
	send(t, c, websocket.MessageBinary, big)
	var got []byte
	for len(got) < len(want) {
		typ, m, err := read(t, c, 5*time.Second)
		if err != nil || typ != websocket.MessageBinary {
			t.Fatalf("after %d bytes: %v %v", len(got), typ, err)
		}
		got = append(got, m...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stream came back changed (%d bytes)", len(got))
	}
	c.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(2 * time.Second)
	for !logs.has("tcp close") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !logs.has(fmt.Sprintf("up=%d down=%d", len(want), len(want))) {
		t.Fatalf("close line does not count the bytes: %q", logs.lines)
	}
}

func TestTCPServerCloseEndsSocket(t *testing.T) {
	ln := tcpServer(t, func(c net.Conn) { c.Write([]byte("hi")) })
	_, srv, _ := newRelay(t, Config{AllowTCP: allow(t, fmt.Sprintf("127.0.0.1:%d", lnPort(ln)))})
	c := dialOK(t, srv, fmt.Sprintf("/tcp?host=127.0.0.1&port=%d", lnPort(ln)))
	readText(t, c)
	if _, m, err := read(t, c, 5*time.Second); err != nil || string(m) != "hi" {
		t.Fatalf("got %q %v", m, err)
	}
	_, _, err := read(t, c, 5*time.Second)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("the server's close should end the socket cleanly, got %v", err)
	}
}

func TestTCPRefused(t *testing.T) {
	ln := tcpServer(t, echoServe)
	var reached atomic.Int64
	other := tcpServer(t, func(net.Conn) { reached.Add(1) })
	_, srv, logs := newRelay(t, Config{AllowTCP: allow(t, fmt.Sprintf("127.0.0.1:%d", lnPort(ln)))})

	// positive control
	c := dialOK(t, srv, fmt.Sprintf("/tcp?host=127.0.0.1&port=%d", lnPort(ln)))
	okPort(t, readText(t, c), "127.0.0.1")

	for _, q := range []string{
		fmt.Sprintf("host=127.0.0.1&port=%d", lnPort(other)), // listening, not allowed
		fmt.Sprintf("host=localhost&port=%d", lnPort(ln)),    // a name, not an address
		"host=127.0.0.1&port=abc",
		"host=127.0.0.1&port=0",
		"",
	} {
		c := dialOK(t, srv, "/tcp?"+q)
		msg := readText(t, c)
		if !strings.HasPrefix(msg, "err ") {
			t.Fatalf("%s: got %q, want err", q, msg)
		}
		if _, _, err := read(t, c, 5*time.Second); websocket.CloseStatus(err) == -1 {
			t.Fatalf("%s: socket not closed after err: %v", q, err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if reached.Load() != 0 {
		t.Fatal("the relay connected to a port it was not given")
	}
	if !logs.has("tcp refused") {
		t.Fatal("refusals are not logged")
	}
}

func TestOrigin(t *testing.T) {
	echo := newUDPPeer(t, true)
	_, srv, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String()), Origins: []string{"https://good.example"}})
	h := func(o string) http.Header { return http.Header{"Origin": {o}} }

	_, resp, err := dial(t, srv, "/udp", h("https://evil.example"))
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a wrong origin should get 403, got %v %v", resp, err)
	}
	for _, o := range []string{"https://good.example", "HTTPS://GOOD.EXAMPLE/"} {
		c, _, err := dial(t, srv, "/udp", h(o))
		if err != nil {
			t.Fatalf("origin %s refused: %v", o, err)
		}
		readText(t, c)
	}
	// no Origin header: not a browser page, let through
	c, _, err := dial(t, srv, "/udp", nil)
	if err != nil {
		t.Fatalf("no origin refused: %v", err)
	}
	readText(t, c)

	// negative control: with no -origin, any page may connect
	_, open, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String())})
	c, _, err = dial(t, open, "/udp", h("https://evil.example"))
	if err != nil {
		t.Fatalf("with no origins configured, an origin was refused: %v", err)
	}
	readText(t, c)
}

func TestPerIPLimit(t *testing.T) {
	echo := newUDPPeer(t, true)
	r, srv, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String()), MaxPerIP: 2})
	a := dialOK(t, srv, "/udp")
	readText(t, a)
	b := dialOK(t, srv, "/udp")
	readText(t, b)
	if _, resp, err := dial(t, srv, "/udp", nil); err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("a third socket should get 429, got %v %v", resp, err)
	}
	a.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(3 * time.Second)
	for r.Held("127.0.0.1") != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := r.Held("127.0.0.1"); n != 1 {
		t.Fatalf("after a close, %d sockets held, want 1", n)
	}
	c := dialOK(t, srv, "/udp")
	readText(t, c)

	// behind a proxy on loopback each client is counted on its own
	r1, srv1, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String()), MaxPerIP: 1})
	ip := func(s string) http.Header { return http.Header{"X-Real-Ip": {s}} }
	for _, s := range []string{"203.0.113.1", "203.0.113.2"} {
		c, _, err := dial(t, srv1, "/udp", ip(s))
		if err != nil {
			t.Fatalf("client %s refused: %v", s, err)
		}
		readText(t, c)
	}
	if _, resp, err := dial(t, srv1, "/udp", ip("203.0.113.1")); err == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("a second socket for 203.0.113.1 should get 429, got %v %v", resp, err)
	}
	if r1.Held("203.0.113.1") != 1 || r1.Held("203.0.113.2") != 1 {
		t.Fatalf("held: %d %d", r1.Held("203.0.113.1"), r1.Held("203.0.113.2"))
	}
}

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		remote, xff, real, want string
	}{
		{"127.0.0.1:5555", "198.51.100.7, 203.0.113.9", "", "203.0.113.9"}, // the proxy's entry is the last
		{"127.0.0.1:5555", "198.51.100.7, 203.0.113.9", "203.0.113.20", "203.0.113.20"},
		{"127.0.0.1:5555", "", "", "127.0.0.1"},
		{"127.0.0.1:5555", "", "garbage", "127.0.0.1"},
		{"198.51.100.1:5555", "203.0.113.9", "203.0.113.20", "198.51.100.1"}, // not a proxy: headers ignored
	} {
		req := httptest.NewRequest("GET", "/udp", nil)
		req.RemoteAddr = tc.remote
		if tc.xff != "" {
			req.Header.Set("X-Forwarded-For", tc.xff)
		}
		if tc.real != "" {
			req.Header.Set("X-Real-IP", tc.real)
		}
		if got := ClientIP(req); got != tc.want {
			t.Errorf("%+v: got %s", tc, got)
		}
	}
}

func TestParseAllow(t *testing.T) {
	a := allow(t, "203.0.113.5:10070", "203.0.113.5:9735-9740,198.51.100.0/24:7000", "localhost:7003")
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"203.0.113.5:10070", true},
		{"203.0.113.5:10071", false},
		{"203.0.113.6:10070", false},
		{"203.0.113.5:9735", true},
		{"203.0.113.5:9740", true},
		{"203.0.113.5:9741", false},
		{"198.51.100.200:7000", true},
		{"198.51.100.200:7001", false},
		{"198.51.101.1:7000", false},
		{"127.0.0.1:7003", true},
		{"127.0.0.1:7004", false},
	} {
		ap := netip.MustParseAddrPort(tc.addr)
		if got := a.Allows(ap.Addr(), ap.Port()); got != tc.want {
			t.Errorf("%s: %v", tc.addr, got)
		}
	}
	if f, ok := a.First(); !ok || f.String() != "203.0.113.5" {
		t.Errorf("First: %v %v", f, ok)
	}
	for _, bad := range []string{"203.0.113.5", "203.0.113.5:", ":80", "203.0.113.5:0", "203.0.113.5:9-8", "203.0.113.5:70000", "2001:db8::1:80", "203.0.113.0/33:80", "foo.invalid:80"} {
		if _, err := ParseAllow([]string{bad}); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestUDPIdle(t *testing.T) {
	echo := newUDPPeer(t, true)
	_, srv, logs := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String()), UDPIdle: 200 * time.Millisecond})

	// kept busy, it stays open past the idle time
	busy := dialOK(t, srv, "/udp")
	readText(t, busy)
	for i := 0; i < 8; i++ {
		send(t, busy, websocket.MessageBinary, dgram(echo.addr(), "tick"))
		if _, _, err := read(t, busy, time.Second); err != nil {
			t.Fatalf("busy socket closed after %d ticks: %v", i, err)
		}
		time.Sleep(80 * time.Millisecond)
	}

	idle := dialOK(t, srv, "/udp")
	readText(t, idle)
	start := time.Now()
	_, _, err := read(t, idle, 3*time.Second)
	if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("an idle socket should be closed cleanly, got %v", err)
	}
	if el := time.Since(start); el < 150*time.Millisecond || el > 2*time.Second {
		t.Fatalf("closed after %v, want about 200 ms", el)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !logs.has("(idle)") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !logs.has("(idle)") {
		t.Fatalf("no idle close line: %q", logs.lines)
	}
}

func TestPingDropsSilentClient(t *testing.T) {
	echo := newUDPPeer(t, true)
	r, srv, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String()), Ping: 50 * time.Millisecond})

	// a client that reads answers the pings and stays
	alive := dialOK(t, srv, "/udp")
	readText(t, alive)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readErr := make(chan error, 1)
	go func() {
		_, _, err := alive.Read(ctx)
		readErr <- err
	}()

	// a client that never reads never answers
	silent := dialOK(t, srv, "/udp")
	readText(t, silent)

	deadline := time.Now().Add(3 * time.Second)
	for r.Held("127.0.0.1") != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := r.Held("127.0.0.1"); n != 1 {
		t.Fatalf("%d sockets held, want 1 (the silent one dropped)", n)
	}
	select {
	case err := <-readErr:
		t.Fatalf("the answering client was dropped: %v", err)
	default:
	}
}

func TestHTTPEndpoints(t *testing.T) {
	_, srv, _ := newRelay(t, Config{AllowUDP: allow(t, "127.0.0.1:9")})
	get := func(path string) (int, string, string) {
		t.Helper()
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header.Get("Access-Control-Allow-Origin")
	}
	if code, body, cors := get("/resolve?name=localhost"); code != 200 || body != "127.0.0.1\n" || cors != "*" {
		t.Errorf("/resolve localhost: %d %q cors %q", code, body, cors)
	}
	if code, _, _ := get("/resolve?name=no-such-host.invalid"); code != 404 {
		t.Errorf("/resolve of nothing: %d", code)
	}
	if code, body, cors := get("/local?to=127.0.0.1"); code != 200 || body != "127.0.0.1\n" || cors != "*" {
		t.Errorf("/local: %d %q %q", code, body, cors)
	}
	if code, body, _ := get("/local?to=garbage"); code != 200 || body != "127.0.0.1\n" {
		t.Errorf("/local garbage: %d %q", code, body)
	}
	if code, _, _ := get("/elsewhere"); code != 404 {
		t.Errorf("/elsewhere: %d", code)
	}
	// a plain GET of a socket path is not an upgrade
	if code, _, _ := get("/udp"); code == 200 || code == 101 {
		t.Errorf("/udp without an upgrade: %d", code)
	}
}

// ClientOf / Lookup: the relayed server sees the relay's address, and the
// relay names the page's behind each local port, for as long as the socket
// is open. Control: a port the relay does not hold is unknown.
func TestClientOfNamesThePage(t *testing.T) {
	echo := newUDPPeer(t, true)
	r, srv, _ := newRelay(t, Config{AllowUDP: allow(t, echo.addr().String())})
	c, _, err := dial(t, srv, "/udp", http.Header{"X-Real-Ip": {"198.51.100.23"}})
	if err != nil {
		t.Fatal(err)
	}
	port := okPort(t, readText(t, c), "127.0.0.1")
	if ip, ok := r.ClientOf(port); !ok || ip != "198.51.100.23" {
		t.Fatalf("ClientOf(%d) = %q %v, want the page's address", port, ip, ok)
	}
	look := httptest.NewServer(r.Lookup())
	t.Cleanup(look.Close)
	get := func(p int) (int, string) {
		resp, err := http.Get(fmt.Sprintf("%s/client?port=%d", look.URL, p))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get(port); code != 200 || body != "198.51.100.23" {
		t.Fatalf("lookup %d: %d %q", port, code, body)
	}
	if code, _ := get(port + 1); code != 404 {
		t.Fatalf("a port the relay does not hold: %d, want 404", code)
	}
	c.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := r.ClientOf(port); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the port still names the page after its socket closed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
